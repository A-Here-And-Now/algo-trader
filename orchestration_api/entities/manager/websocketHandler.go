package manager

import (
	"log"
	"net/http"
	"time"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
	"github.com/gorilla/websocket"
)

type WSMessage struct {
	Type    string   `json:"type"`
	Symbols []string `json:"symbols"`
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WebSocketHandler handles frontend websocket connections
func (m *Manager) WebSocketHandler(w http.ResponseWriter, r *http.Request) {
	// Check if already connected because we really shouldn't allow more than one ever, not designed for multiple users
	m.frontendMutex.Lock()
	if m.frontendConnected {
		m.frontendMutex.Unlock()
		http.Error(w, "Frontend already connected", http.StatusConflict)
		return
	}
	m.frontendConnected = true
	m.frontendMutex.Unlock()

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] upgrade error: %v", err)
		m.frontendMutex.Lock()
		m.frontendConnected = false
		m.frontendMutex.Unlock()
		return
	}
	defer func() {
		conn.Close()
		m.frontendMutex.Lock()
		m.frontendConnected = false
		m.frontendMutex.Unlock()
	}()

	done := make(chan struct{})

	// Goroutine to handle incoming subscription changes
	go func() {
		defer close(done)
		for {
			var msg WSMessage
			if err := conn.ReadJSON(&msg); err != nil {
				log.Printf("[WS] read error: %v", err)
				return
			}

			switch msg.Type {
			case "subscribe":
				for _, symbol := range msg.Symbols {
					log.Printf("[WS] Subscribing to: %+v", msg.Symbols)
					
					// Subscribe to the exchange for this symbol
					// Default to 5m candles for now
					_, _, _, err := m.exchange.SubscribeToCandle(symbol, enum.CandleSize5m)
					if err != nil {
						log.Printf("[WS] failed to subscribe to candles: %v", err)
						continue
					}
					
					// Send historical data
					priceHistory := m.exchange.GetPriceHistory(symbol)
					for _, data := range priceHistory {
						msg := models.GetFrontEndTicker(data)
						if err := conn.WriteJSON(msg); err != nil {
							log.Printf("[WS] write error: %v", err)
						}
					}
					
					candleHistory := m.exchange.GetCandleHistory(symbol, enum.CandleSize5m)
					for _, data := range candleHistory.Candles {
						msg := data.GetFrontEndCandle()
						if err := conn.WriteJSON(msg); err != nil {
							log.Printf("[WS] write error: %v", err)
						}
					}
				}
				log.Printf("[WS] Subscribed to: %+v", msg.Symbols)

			case "unsubscribe":
				// Unsubscribe handled by cleanup functions from exchange subscriptions
				log.Printf("[WS] Unsubscribed from: %+v", msg.Symbols)
			}
		}
	}()

	// Ping ticker
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	// Track subscribed symbols for the frontend
	subscribedSymbols := make(map[string]bool)

	// Main data pump
	for {
		select {
		case <-done:
			return

		case <-pingTicker.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

		default:
			// Send price/candle/order data for subscribed symbols
			for symbol := range subscribedSymbols {
				// Try to read from frontend feeds
				priceCh := m.exchange.GetFrontendPriceFeed(symbol)
				candleCh := m.exchange.GetFrontendCandleFeed(symbol)
				orderCh := m.exchange.GetFrontendOrderFeed(symbol)
				
				select {
				case ticker, ok := <-priceCh:
					if ok {
						msg := models.GetFrontEndTicker(ticker)
						if err := conn.WriteJSON(msg); err != nil {
							log.Printf("[WS] write error: %v", err)
						}
					}
				case candle, ok := <-candleCh:
					if ok {
						msg := candle.GetFrontEndCandle()
						if err := conn.WriteJSON(msg); err != nil {
							log.Printf("[WS] write error: %v", err)
						}
					}
				case ord, ok := <-orderCh:
					if ok {
						if err := conn.WriteJSON(ord); err != nil {
							log.Printf("[WS] write error: %v", err)
						}
					}
				default:
				}
			}

			// Small sleep to prevent busy loop
			time.Sleep(250 * time.Millisecond)
		}
	}
}


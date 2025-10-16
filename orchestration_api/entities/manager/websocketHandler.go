package manager

import (
	"log"
	"net/http"

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

					m.exchange.StartNewTokenDataStream(symbol, enum.CandleSize5m)
					
					// Send historical data
					priceHistory := m.exchange.GetPriceHistory(symbol)
					if len(priceHistory) > 0 {
						msg := models.GetFrontEndTicker(priceHistory[len(priceHistory)-1])
						if err := conn.WriteJSON(msg); err != nil {
							log.Printf("[WS] write error: %v", err)
						}
					}

					candleHistory := m.exchange.GetCandleHistory(symbol)
					if len(candleHistory.Candles) > 0 {	
						for _, data := range candleHistory.Candles {
							msg := data.GetFrontEndCandle()
							if err := conn.WriteJSON(msg); err != nil {
								log.Printf("[WS] write error: %v", err)
							}
						}
					}
					
					candleCh, candleCleanup := m.exchange.SubscribeToCandle(symbol)
					priceCh, priceCleanup := m.exchange.SubscribeToTicker(symbol)
					orderCh, orderCleanup := m.exchange.SubscribeToOrderUpdates(symbol)

					go func(candleCh <-chan models.Candle, candleCleanup func(), priceCh <-chan models.Ticker, priceCleanup func(), orderCh <-chan models.OrderUpdate, orderCleanup func()) {
						defer func() {
							if candleCh != nil {
								candleCleanup()
							}
							if priceCh != nil {
								priceCleanup()
							}
							if orderCh != nil {
								orderCleanup()
							}
						}()
						for {
							select {
								
							case candle, ok := <-candleCh:
								if !ok {
									candleCh = nil  // Prevent this case from being selected again
									continue
								}
								msg := candle.GetFrontEndCandle()
								if err := conn.WriteJSON(msg); err != nil {
									log.Printf("[WS] write error: %v", err)
									return  // Exit on write error
								}
								
							case price, ok := <-priceCh:
								if !ok {
									priceCh = nil
									continue
								}
								msg := models.GetFrontEndTicker(price)
								if err := conn.WriteJSON(msg); err != nil {
									log.Printf("[WS] write error: %v", err)
									return
								}
								
							case order, ok := <-orderCh:
								if !ok {
									orderCh = nil
									continue
								}
								if err := conn.WriteJSON(order); err != nil {
									log.Printf("[WS] write error: %v", err)
									return
								}

							case <-m.ctx.Done():
								return
							}
							
							if candleCh == nil && priceCh == nil && orderCh == nil {
								return
							}
						}
					}(candleCh, candleCleanup, priceCh, priceCleanup, orderCh, orderCleanup)
				}
				log.Printf("[WS] Subscribed to: %+v", msg.Symbols)

			case "unsubscribe":
				// Unsubscribe handled by cleanup functions from exchange subscriptions
				log.Printf("[WS] Unsubscribed from: %+v", msg.Symbols)

				for _, symbol := range msg.Symbols {
					m.Stop(symbol)
					m.exchange.StopTokenDataStream(symbol)
				}
			}
		}
	}()
}

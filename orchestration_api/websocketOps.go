package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

func (m *Manager) startCoinbaseFeed(ctx context.Context, cbAdvUrl string) {
	u, _ := url.Parse(cbAdvUrl)
	go m.runMarketDataWebSocket(ctx, u.String())
}

// runWebSocket manages the lifecycle: connect, read messages, reconnect with backoff if needed.
func (m *Manager) runMarketDataWebSocket(ctx context.Context, wsURL string) {
	backoff := 1 * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		d := websocket.Dialer{
			HandshakeTimeout: 10 * time.Second,
			// TLS / proxy / config options here (InsecureSkipVerify etc) if needed.
		}

		// DialContext will respect ctx cancelation.
		conn, resp, err := d.DialContext(ctx, wsURL, nil)
		if err != nil {
			if resp != nil {
				log.Printf("websocket dial failed, status=%d: %v", resp.StatusCode, err)
			} else {
				log.Printf("websocket dial failed: %v", err)
			}
			// Exponential backoff with cap
			time.Sleep(backoff)
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			continue
		}

		// Enable automatic ping/pong handling (library does it for us)
		conn.SetPongHandler(func(appData string) error { return nil })

		// Subscribe to all tokens
		m.subscribeToAllTokens(conn)

		// Reset backoff after successful connect
		backoff = 1 * time.Second
		log.Printf("websocket connected")

		// Run read pump until error or ctx canceled
		done := make(chan struct{})
		go func() {
			defer close(done)
			m.readLoop(conn)
		}()

		// Optionally: start a ping loop to keep connection alive (some servers require it).
		go pingLoop(conn, ctx)

		go func() {
			for {
				select {
				case symbol := <-m.subscriptionChannel:
					m.subscribeToNewToken(conn, symbol)
				case <-ctx.Done():
					return
				case <-done: // Connection closed
					return
				}
			}
		}()

		// Wait until readLoop exits or context canceled
		select {
		case <-ctx.Done():
			_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "client shutdown"))
			_ = conn.Close()
			<-done
			return
		case <-done:
			// connection closed by readLoop; attempt to reconnect
			_ = conn.Close()
			log.Printf("websocket disconnected; will attempt reconnect")
			// short sleep before reconnect
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// readLoop reads messages from the websocket and processes them.
func (m *Manager) readLoop(conn *websocket.Conn) {
	for {
		// Read a raw JSON message
		_, raw, err := conn.ReadMessage()
		if err != nil {
			// Normal closure (client or server closed) yields an error;
			// we simply exit the goroutine.
			log.Printf("[WS] read error: %v", err)
			return
		}

		// First, peek at the "type" field to decide how to unmarshal.
		var channelType struct {
			Channel string `json:"channel"`
		}
		if err := json.Unmarshal(raw, &channelType); err != nil {
			log.Printf("[WS] malformed message: %v", err)
			continue
		}

		switch channelType.Channel {
		case "ticker_batch":
			var t TickerMsg
			if err := json.Unmarshal(raw, &t); err != nil {
				log.Printf("[WS] ticker unmarshal error: %v", err)
				continue
			}
			// non‑blocking send – drop if buffer full (price is high‑rate)
			for _, event := range t.Events {
				for _, ticker := range event.Tickers {
					m.writePrice(ticker)
				}
			}

		case "candles":
			var c CandleMsg
			if err := json.Unmarshal(raw, &c); err != nil {
				log.Printf("[WS] candle unmarshal error: %v", err)
				continue
			}
			// Coinbase returns an array of candles; we push each one.
			for _, event := range c.Events {
				for _, candle := range event.Candles {
					m.writeCandle(candle)
				}
			}
		// Coinbase also sends keep‑alive messages like {"type":"heartbeat"}
		// – we just ignore them.
		default:
			// no‑op
		}
	}
}

func (m *Manager) writeCandle(candle Candle) {
	safeMarketPriceResources := m.safeGetMarketPriceResources()
	safeTraderResources := m.safeGetTraderResources()
	if _, ok := safeMarketPriceResources[candle.ProductID]; ok {
		writeToChannelAndBufferLatest(safeMarketPriceResources[candle.ProductID].candleFeed, candle)
		safeMarketPriceResources[candle.ProductID].candleHistory = append(safeMarketPriceResources[candle.ProductID].candleHistory, candle)
		if len(safeMarketPriceResources[candle.ProductID].candleHistory) > 50 {
			safeMarketPriceResources[candle.ProductID].candleHistory = safeMarketPriceResources[candle.ProductID].candleHistory[1:]
		}
	} else {
		log.Printf("writeCandle: %s not found in marketPriceResources", candle.ProductID)
	}
	if _, ok := safeTraderResources[candle.ProductID]; ok {
		writeToChannelAndBufferLatest(safeTraderResources[candle.ProductID].candleFeed, candle)
	} else {
		log.Printf("writeCandle: %s not found in traderResources", candle.ProductID)
	}
}

func (m *Manager) writePrice(ticker Ticker) {
	safeMarketPriceResources := m.safeGetMarketPriceResources()
	safeTraderResources := m.safeGetTraderResources()
	if _, ok := safeMarketPriceResources[ticker.ProductID]; ok {
		writeToChannelAndBufferLatest(safeMarketPriceResources[ticker.ProductID].priceFeed, ticker)
		safeMarketPriceResources[ticker.ProductID].priceHistory = append(safeMarketPriceResources[ticker.ProductID].priceHistory, ticker)
		if len(safeMarketPriceResources[ticker.ProductID].priceHistory) > 50 {
			safeMarketPriceResources[ticker.ProductID].priceHistory = safeMarketPriceResources[ticker.ProductID].priceHistory[1:]
		}
	} else {
		log.Printf("writePrice: %s not found in marketPriceResources", ticker.ProductID)
	}
	if _, ok := safeTraderResources[ticker.ProductID]; ok {
		writeToChannelAndBufferLatest(safeTraderResources[ticker.ProductID].priceFeed, ticker)
	} else {
		log.Printf("writePrice: %s not found in traderResources", ticker.ProductID)
	}
}

// pingLoop optionally sends regular pings to keep the connection alive.
// Some providers send pings themselves, or expect pongs; check your provider's docs.
func pingLoop(conn *websocket.Conn, ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("ping failed: %v", err)
				return
			}
		}
	}
}

func (m *Manager) subscribeToAllTokens(conn *websocket.Conn) {
	// Subscription json to the two channels we need, for all products in the "tokens" array
	subPayload := GetCoinbaseSubscriptionPayload(tokens)
	m.sendPayload(conn, subPayload)
}

func (m *Manager) subscribeToNewToken(conn *websocket.Conn, symbol string) {
	subPayload := GetCoinbaseSubscriptionPayload([]string{symbol})
	m.sendPayload(conn, subPayload)
}

func (m *Manager) sendPayload(conn *websocket.Conn, payload []CoinbaseSubscription) {
	for _, p := range payload {
		if err := conn.WriteJSON(p); err != nil {
			log.Printf("Failed to send subscription: %v", err)
		}
	}
}

type WSMessage struct {
	Type    string   `json:"type"`
	Symbols []string `json:"symbols"`
}

func (m *Manager) wsHandler(w http.ResponseWriter, r *http.Request) {
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
					if _, ok := m.marketPriceResources[symbol]; !ok {
						m.safeAddMarketPriceResource(symbol)
					}
					m.subscriptionChannel <- symbol
					safeMarketPriceResources := m.safeGetMarketPriceResources()
					for _, data := range safeMarketPriceResources[symbol].priceHistory {
						msg := getFrontEndTicker(data)
						if err := conn.WriteJSON(msg); err != nil {
							log.Printf("[WS] write error: %v", err)
						}
					}
					for _, data := range safeMarketPriceResources[symbol].candleHistory {
						msg := getFrontEndCandle(data)
						if err := conn.WriteJSON(msg); err != nil {
							log.Printf("[WS] write error: %v", err)
						}
					}
				}
				log.Printf("[WS] Subscribed to: %+v", msg.Symbols)

			case "unsubscribe":
				for _, symbol := range msg.Symbols {
					m.safeRemoveMarketPriceResource(symbol)
				}
				log.Printf("[WS] Unsubscribed from: %+v", msg.Symbols)
			}
		}
	}()

	// Ping ticker
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

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
			// Check each active subscription
			safeMarketPriceResources := m.safeGetMarketPriceResources()

			// Send price data for subscribed symbols
			for _, resource := range safeMarketPriceResources {
				for {
					select {
					case ticker := <-resource.priceFeed:
						msg := getFrontEndTicker(ticker)
						if err := conn.WriteJSON(msg); err != nil {
							log.Printf("[WS] write error: %v", err)
						}
					case candle := <-resource.candleFeed:
						msg := getFrontEndCandle(candle)
						if err := conn.WriteJSON(msg); err != nil {
							log.Printf("[WS] write error: %v", err)
						}
					default:
						break // No price/candle data available, continue
					}
				}
			}
			// Small sleep to prevent busy loop
			time.Sleep(15 * time.Millisecond)
		}
	}
}

package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/channel_helper"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/coinbase"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
	"github.com/gorilla/websocket"
)

type UserChannelMessage struct {
	Channel string `json:"channel"`
	Events  []struct {
		Type   string           `json:"type"`
		Orders []coinbase.Order `json:"orders"`
	} `json:"events"`
}

// dialWebSocket connects with the JWT included in the HTTP headers for the WebSocket handshake.
func DialWebSocketWithAuth(ctx context.Context, wsURL string, apiKey string, apiSecret string) (*websocket.Conn, *http.Response, error) {
	jwtTok, err := coinbase.BuildJWT(apiKey, apiSecret)
	if err != nil {
		return nil, nil, fmt.Errorf("build jwt: %w", err)
	}

	headers := http.Header{}
	// Most APIs accept an Authorization: Bearer <token> header at handshake time.
	headers.Set("Authorization", "Bearer "+jwtTok)

	// Some APIs expect the token as a query param or Sec-WebSocket-Protocol — check docs.
	// headers.Set("Sec-WebSocket-Protocol", "jwt."+jwtTok)

	d := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		// TLS / proxy / config options here (InsecureSkipVerify etc) if needed.
	}

	// DialContext will respect ctx cancelation.
	conn, resp, err := d.DialContext(ctx, wsURL, headers)
	return conn, resp, err
}

func (m *Manager) StartCoinbaseFeed(ctx context.Context, cbAdvUrl string) {
	u, _ := url.Parse(cbAdvUrl)
	go m.runMarketDataWebSocket(ctx, u.String())
}

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
		m.marketDataWS = conn
		defer func() {
			m.marketDataWS = nil
		}()
		// Enable automatic ping/pong handling (library does it for us)
		conn.SetPongHandler(func(appData string) error { return nil })

		// Subscribe to all tokens
		m.subscribeToMarketDataForAllTokens(conn)

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
		go PingLoop(conn, ctx)

		go func() {
			for {
				select {
				case symbol := <-m.subscriptionChannel:
					m.subscribeToNewToken(symbol)
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
			m.marketDataWS = nil
			<-done
			return
		case <-done:
			// connection closed by readLoop; attempt to reconnect
			_ = conn.Close()
			m.marketDataWS = nil
			log.Printf("websocket disconnected; will attempt reconnect")
			// short sleep before reconnect
			time.Sleep(500 * time.Millisecond)
		}
	}
}

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
			var t models.TickerMsg
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
			var c models.CandleMsg
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

func (m *Manager) writeCandle(candle models.Candle) {
	safeMarketPriceResources := m.safeGetMarketPriceResources()
	safeTraderResources := m.safeGetTraderResources()
	if _, ok := safeMarketPriceResources[candle.ProductID]; ok {
		channel_helper.WriteToChannelAndBufferLatest(safeMarketPriceResources[candle.ProductID].CandleFeed, candle)
		safeMarketPriceResources[candle.ProductID].CandleHistory = append(safeMarketPriceResources[candle.ProductID].CandleHistory, candle)
		if len(safeMarketPriceResources[candle.ProductID].CandleHistory) > 50 {
			safeMarketPriceResources[candle.ProductID].CandleHistory = safeMarketPriceResources[candle.ProductID].CandleHistory[1:]
		}
	} else {
		log.Printf("writeCandle: %s not found in marketPriceResources", candle.ProductID)
	}
	if _, ok := safeTraderResources[candle.ProductID]; ok {
		channel_helper.WriteToChannelAndBufferLatest(safeTraderResources[candle.ProductID].CandleFeedToSignalEngine, candle)
	} else {
		log.Printf("writeCandle: %s not found in traderResources", candle.ProductID)
	}
}

func (m *Manager) writePrice(ticker models.Ticker) {
	safeMarketPriceResources := m.safeGetMarketPriceResources()
	safeTraderResources := m.safeGetTraderResources()
	if _, ok := safeMarketPriceResources[ticker.ProductID]; ok {
		channel_helper.WriteToChannelAndBufferLatest(safeMarketPriceResources[ticker.ProductID].PriceFeed, ticker)
		safeMarketPriceResources[ticker.ProductID].PriceHistory = append(safeMarketPriceResources[ticker.ProductID].PriceHistory, ticker)
		if len(safeMarketPriceResources[ticker.ProductID].PriceHistory) > 50 {
			safeMarketPriceResources[ticker.ProductID].PriceHistory = safeMarketPriceResources[ticker.ProductID].PriceHistory[1:]
		}
	} else {
		log.Printf("writePrice: %s not found in marketPriceResources", ticker.ProductID)
	}
	if _, ok := safeTraderResources[ticker.ProductID]; ok {
		channel_helper.WriteToChannelAndBufferLatest(safeTraderResources[ticker.ProductID].PriceFeedToSignalEngine, ticker)
		channel_helper.WriteToChannelAndBufferLatest(safeTraderResources[ticker.ProductID].PriceFeedToTrader, ticker)
	} else {
		log.Printf("writePrice: %s not found in traderResources", ticker.ProductID)
	}
}

func PingLoop(conn *websocket.Conn, ctx context.Context) {
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

func (m *Manager) subscribeToMarketDataForAllTokens(conn *websocket.Conn) {
	// Subscription json to the two channels we need, for all products in the "tokens" array
	subPayload := coinbase.GetMarketSubscriptionPayload(m.tokens)
	for _, p := range subPayload {
		if err := m.marketDataWS.WriteJSON(p); err != nil {
			log.Printf("Failed to send subscription: %v", err)
		}
	}
}

func (m *Manager) subscribeToNewToken(symbol string) {
	marketDataSubPayload := coinbase.GetMarketSubscriptionPayload([]string{symbol})
	for _, p := range marketDataSubPayload {
		if err := m.marketDataWS.WriteJSON(p); err != nil {
			log.Printf("Failed to send subscription: %v", err)
		}
	}
	userDataSubPayload, err := m.GetUserDataSubscriptionPayload([]string{symbol})
	if err != nil {
		log.Printf("failed to get user data subscription payload: %v", err)
		return
	}
	if err := m.userDataWS.WriteJSON(userDataSubPayload); err != nil {
		log.Printf("Failed to send subscription: %v", err)
	}
}

type WSMessage struct {
	Type    string   `json:"type"`
	Symbols []string `json:"symbols"`
}

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
					if _, ok := m.marketPriceResources[symbol]; !ok {
						candles, err := m.client.GetHistoricalCandles(m.ctx, symbol)
						if err != nil {
							log.Printf("[WS] get historical candles error: %v", err)
							continue
						}
						m.safeAddMarketPriceResource(symbol, models.GetCandles(candles.Candles, symbol))
					}
					m.subscriptionChannel <- symbol
					safeMarketPriceResources := m.safeGetMarketPriceResources()
					for _, data := range safeMarketPriceResources[symbol].PriceHistory {
						msg := models.GetFrontEndTicker(data)
						if err := conn.WriteJSON(msg); err != nil {
							log.Printf("[WS] write error: %v", err)
						}
					}
					for _, data := range safeMarketPriceResources[symbol].CandleHistory {
						msg := data.GetFrontEndCandle()
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
				select {
				case ticker := <-resource.PriceFeed:
					msg := models.GetFrontEndTicker(ticker)
					if err := conn.WriteJSON(msg); err != nil {
						log.Printf("[WS] write error: %v", err)
					}
				case candle := <-resource.CandleFeed:
					msg := candle.GetFrontEndCandle()
					if err := conn.WriteJSON(msg); err != nil {
						log.Printf("[WS] write error: %v", err)
					}
				case ord := <-resource.OrderFeed:
					if err := conn.WriteJSON(ord); err != nil {
						log.Printf("[WS] write error: %v", err)
					}
				default:
				}
			}

			// Small sleep to prevent busy loop
			time.Sleep(250 * time.Millisecond)
		}
	}
}

func (m *Manager) StartOrderAndPositionValuationWebSocket(ctx context.Context, wsURL string) {
	go m.runUserWebSocket(ctx, wsURL)
}

func (m *Manager) runUserWebSocket(ctx context.Context, wsURL string) {
	backoff := 1 * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Dial with JWT auth header
		conn, resp, err := DialWebSocketWithAuth(ctx, wsURL, m.apiKey, m.apiSecret)
		if err != nil {
			if resp != nil {
				log.Printf("user websocket dial failed, status=%d: %v", resp.StatusCode, err)
			} else {
				log.Printf("user websocket dial failed: %v", err)
			}
			time.Sleep(backoff)
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			continue
		}

		m.userDataWS = conn
		defer func() {
			m.userDataWS = nil
		}()
		// Send subscription messages for orders and positions per product
		if err := m.sendUserSubscriptions(conn); err != nil {
			log.Printf("failed to subscribe on user ws: %v", err)
			_ = conn.Close()
			time.Sleep(2 * time.Second)
			continue
		}

		backoff = 1 * time.Second
		log.Printf("user websocket connected")

		done := make(chan struct{})
		go func() {
			defer close(done)
			m.readUserLoop(conn)
		}()

		go PingLoop(conn, ctx)

		select {
		case <-ctx.Done():
			_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "client shutdown"))
			_ = conn.Close()
			m.userDataWS = nil
			<-done
			return
		case <-done:
			_ = conn.Close()
			m.userDataWS = nil
			log.Printf("user websocket disconnected; will attempt reconnect")
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func (m *Manager) sendUserSubscriptions(conn *websocket.Conn) error {
	sub, err := m.GetUserDataSubscriptionPayload(m.tokens)
	if err != nil {
		return err
	}
	return conn.WriteJSON(sub)
}

func (m *Manager) GetUserDataSubscriptionPayload(tokens []string) (map[string]any, error) {
	// Subscribe once to unified "user" channel for all products, include jwt in payload
	jwt, err := coinbase.BuildJWT(m.apiKey, m.apiSecret)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"type":        "subscribe",
		"channel":     "user",
		"product_ids": tokens,
		"jwt":         jwt,
	}, nil
}

func (m *Manager) readUserLoop(conn *websocket.Conn) {
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[UserWS] read error: %v", err)
			return
		}

		var msg UserChannelMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			log.Printf("[UserWS] malformed user message: %v", err)
			continue
		}
		if msg.Channel != "user" {
			continue
		}
		for _, ev := range msg.Events {
			for _, o := range ev.Orders {
				update := o.ToOrderUpdate()
				m.routeOrderUpdate(update)
			}
		}
	}
}

func (m *Manager) routeOrderUpdate(up coinbase.OrderUpdate) {
	safeMarketPriceResources := m.safeGetMarketPriceResources()
	safeTraderResources := m.safeGetTraderResources()
	if _, ok := safeMarketPriceResources[up.ProductID]; ok {
		channel_helper.WriteToChannelAndBufferLatest(safeMarketPriceResources[up.ProductID].OrderFeed, up)
	} else {
		log.Printf("routeOrderUpdate: %s not found in marketPriceResources", up.ProductID)
	}
	if _, ok := safeTraderResources[up.ProductID]; ok {
		channel_helper.WriteToChannelAndBufferLatest(safeTraderResources[up.ProductID].OrderFeed, up)
	} else {
		log.Printf("routeOrderUpdate: %s not found in traderResources", up.ProductID)
	}
}

package coinbase

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/channel_helper"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
	cb_models "github.com/A-Here-And-Now/algo-trader/orchestration_api/models/coinbase"
	"github.com/gorilla/websocket"
)

type UserChannelMessage struct {
	Channel string `json:"channel"`
	Events  []struct {
		Type   string            `json:"type"`
		Orders []cb_models.Order `json:"orders"`
	} `json:"events"`
}

// dialWebSocket connects with the JWT included in the HTTP headers for the WebSocket handshake.
func dialWebSocketWithAuth(ctx context.Context, wsURL string, apiKey string, apiSecret string) (*websocket.Conn, *http.Response, error) {
	jwtTok, err := buildJWT(apiKey, apiSecret)
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

func (e *CoinbaseExchange) runMarketDataWebSocket(ctx context.Context, wsURL string) {
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
		e.mu.Lock()
		e.marketDataWS = conn
		e.mu.Unlock()
		defer func() {
			e.mu.Lock()
			e.marketDataWS = nil
			e.mu.Unlock()
		}()
		// Enable automatic ping/pong handling (library does it for us)
		conn.SetPongHandler(func(appData string) error { return nil })

		// Subscribe to all tokens
		e.subscribeToMarketDataForAllTokens(conn)

		// Reset backoff after successful connect
		backoff = 1 * time.Second
		log.Printf("websocket connected")

		// Run read pump until error or ctx canceled
		done := make(chan struct{})
		go func() {
			defer close(done)
			e.readLoop(conn)
		}()

		// Optionally: start a ping loop to keep connection alive (some servers require it).
		go pingLoop(conn, ctx)

		// Wait until readLoop exits or context canceled
		select {
		case <-ctx.Done():
			_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "client shutdown"))
			_ = conn.Close()
			e.mu.Lock()
			e.marketDataWS = nil
			e.mu.Unlock()
			<-done
			return
		case <-done:
			// connection closed by readLoop; attempt to reconnect
			_ = conn.Close()
			e.mu.Lock()
			e.marketDataWS = nil
			e.mu.Unlock()
			log.Printf("websocket disconnected; will attempt reconnect")
			// short sleep before reconnect
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func (e *CoinbaseExchange) readLoop(conn *websocket.Conn) {
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
		case "candles":
			var c models.CandleMsg
			if err := json.Unmarshal(raw, &c); err != nil {
				log.Printf("[WS] candle unmarshal error: %v", err)
				continue
			}

			for _, event := range c.Events {
				for _, coinbaseCandle := range event.Candles {
					candle := coinbaseCandle.ToCandle()
					e.consumeCandle(candle)
				}
			}
		// Coinbase also sends keep‑alive messages like {"type":"heartbeat"}
		// – we just ignore them.
		default:
			// no‑op
		}
	}
}

func (e *CoinbaseExchange) consumeCandle(inboundCandle models.Candle) {
	candleToPublish := e.priceActionStore.IngestCandleOfInboundCandleSize(inboundCandle)
	e.publishCandle(candleToPublish)
	e.publishPrice(candleToPublish)
}

func (e *CoinbaseExchange) publishCandle(candle models.Candle) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Write to all subscribed candle channels for this symbol and size
	if channels, ok := e.candleChannels[candle.ProductID]; ok {
		for _, ch := range channels {
			channel_helper.WriteToChannelAndBufferLatest(ch, candle)
		}
	}
}

func (e *CoinbaseExchange) publishPrice(candle models.Candle) {
	e.mu.Lock()
	defer e.mu.Unlock()

	symbol := candle.ProductID

	ticker := models.Ticker{
		Symbol: symbol,
		Price:  candle.Close,
		Time:   time.Now(),
	}

	// Write to all subscribed ticker channels for this symbol
	if channels, ok := e.tickerChannels[symbol]; ok {
		for _, ch := range channels {
			channel_helper.WriteToChannelAndBufferLatest(ch, ticker)
		}
	}
}

func (e *CoinbaseExchange) subscribeToMarketDataForAllTokens(conn *websocket.Conn) {
	e.mu.Lock()
	// Get all currently subscribed symbols
	symbols := make([]string, 0, len(e.symbolSubscriptions))
	for symbol := range e.symbolSubscriptions {
		symbols = append(symbols, symbol)
	}
	e.mu.Unlock()

	if len(symbols) == 0 {
		return
	}

	// Subscription json to the two channels we need, for all products in the "tokens" array
	subPayload := cb_models.GetMarketSubscriptionPayload(symbols, false)
	for _, p := range subPayload {
		if err := conn.WriteJSON(p); err != nil {
			log.Printf("Failed to send subscription: %v", err)
		}
	}
}

func (e *CoinbaseExchange) runUserWebSocket(ctx context.Context, wsURL string) {
	backoff := 1 * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Dial with JWT auth header
		conn, resp, err := dialWebSocketWithAuth(ctx, wsURL, e.apiKey, e.apiSecret)
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

		e.mu.Lock()
		e.userDataWS = conn
		e.mu.Unlock()
		defer func() {
			e.mu.Lock()
			e.userDataWS = nil
			e.mu.Unlock()
		}()
		// Send subscription messages for orders and positions per product
		if err := e.sendUserSubscriptions(conn); err != nil {
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
			e.readUserLoop(conn)
		}()

		go pingLoop(conn, ctx)

		select {
		case <-ctx.Done():
			_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "client shutdown"))
			_ = conn.Close()
			e.mu.Lock()
			e.userDataWS = nil
			e.mu.Unlock()
			<-done
			return
		case <-done:
			_ = conn.Close()
			e.mu.Lock()
			e.userDataWS = nil
			e.mu.Unlock()
			log.Printf("user websocket disconnected; will attempt reconnect")
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func (e *CoinbaseExchange) sendUserSubscriptions(conn *websocket.Conn) error {
	e.mu.Lock()
	symbols := make([]string, 0, len(e.symbolSubscriptions))
	for symbol := range e.symbolSubscriptions {
		symbols = append(symbols, symbol)
	}
	e.mu.Unlock()

	if len(symbols) == 0 {
		return nil
	}

	sub, err := e.getUserDataSubscriptionPayload(symbols, false)
	if err != nil {
		return err
	}
	return conn.WriteJSON(sub)
}

func (e *CoinbaseExchange) getUserDataSubscriptionPayload(tokens []string, isUnsubscribe bool) (map[string]any, error) {
	subType := "subscribe"
	if isUnsubscribe {
		subType = "unsubscribe"
	}
	// Subscribe once to unified "user" channel for all products, include jwt in payload
	jwt, err := buildJWT(e.apiKey, e.apiSecret)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"type":        subType,
		"channel":     "user",
		"product_ids": tokens,
		"jwt":         jwt,
	}, nil
}

func (e *CoinbaseExchange) readUserLoop(conn *websocket.Conn) {
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
				update := getOrderUpdate(o)
				e.publishOrderUpdate(update)
			}
		}
	}
}

func getOrderUpdate(o cb_models.Order) models.OrderUpdate {
	return models.OrderUpdate{
		Channel:       "user",
		ProductID:     o.ProductID,
		OrderID:       o.OrderID,
		Status:        o.Status,
		FilledQty:     o.CumulativeQuantity,
		FilledValue:   o.FilledValue,
		CompletionPct: o.CompletionPct,
		Leaves:        o.Leaves,
		Price:         o.AvgPrice,
		Side:          o.OrderSide,
		Ts:            models.GetTimeFromUnixTimestamp(o.CreationTime),
	}
}

func (e *CoinbaseExchange) publishOrderUpdate(up models.OrderUpdate) {
	e.mu.Lock()
	defer e.mu.Unlock()

	symbol := up.ProductID

	// Write to all subscribed order channels for this symbol
	if channels, ok := e.orderChannels[symbol]; ok {
		for _, ch := range channels {
			channel_helper.WriteToChannelAndBufferLatest(ch, up)
		}
	}
}

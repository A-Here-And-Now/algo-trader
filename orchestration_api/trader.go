// trader.go
package main

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

// TradeCfg contains whatever parameters your algorithm needs.
// Keep it small for the example – you can expand it as you wish.
type TradeCfg struct {
	Symbol         string   `json:"symbol"`   // e.g. "BTCUSD"
	AllocatedFunds float64  `json:"size"`     // position size
	Strategy       Strategy `json:"strategy"` // trading strategy
}

type Trader struct {
	cfg     TradeCfg
	ctx     context.Context
	cancel  context.CancelFunc
	updates chan TradeCfg
	signalCh chan Signal
	actualPositionUSD                float64 // actual position in USD
	actualPositionUSDNoGainsOrLosses float64 // actual position in USD without gains or losses
	targetPositionUSD                float64 // target position in USD
	orderFeed     chan OrderUpdate
	pendingOrder  PendingOrder
	client        *CoinbaseClient
}

type PendingOrder struct {
	OrderID string
	SubmitTime time.Time
	OrderType SignalType
	AmountInUSD float64
}

func (t *Trader) getTargetPositionPct() float64 {
	return t.targetPositionUSD / t.cfg.AllocatedFunds
}

func (t *Trader) getActualPositionPct() float64 {
	return t.actualPositionUSD / t.cfg.AllocatedFunds
}

// dialWebSocket connects with the JWT included in the HTTP headers for the WebSocket handshake.
func dialWebSocketWithAuth(ctx context.Context, wsURL string) (*websocket.Conn, *http.Response, error) {
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

func buildJWT(apiKey, apiSecret string) (string, error) {
	// Typical claims: iat (issued at), exp (expiry), sub (subject) or apikey
	apiKey = os.Getenv("COINBASE_API_KEY")

	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"iat": now.Unix(),
		"exp": now.Add(2 * time.Minute).Unix(), // short-lived token
		"sub": apiKey,                          // example — use actual claim names required by provider
		// add other claims required by the API (e.g., "kid", "scope", "aud", etc.)
	}

	// Coinbase advanced trade user websocket expects ES256 with your API private key.
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	block, _ := pem.Decode([]byte(os.Getenv("COINBASE_PRIVATE_KEY_PEM")))
	if block == nil {
		log.Fatal("failed to decode PEM block")
	}

	privKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		log.Fatalf("failed to parse EC private key: %v", err)
	}

	// If the API requires a keyed HMAC signature but with a hex key, you might compute it differently.
	// The simple case: use the API secret bytes as the HMAC key.
	return token.SignedString(privKey)
}

// NewTrader builds a trader instance from a config.
func NewTrader(cfg TradeCfg, ctx context.Context, cancel context.CancelFunc, updates chan TradeCfg, signalCh chan Signal, orderFeed chan OrderUpdate) *Trader {
	return &Trader{cfg: cfg, ctx: ctx, cancel: cancel, updates: updates, signalCh: signalCh, orderFeed: orderFeed}
}

// Run is the long‑running loop that talks to an exchange, processes signals, etc.
// It stops when ctx is cancelled.
func (t *Trader) Run() {
	log.Printf("[Trader %s] started – AllocatedFunds=%v", t.cfg.Symbol, t.cfg.AllocatedFunds)
	// On startup, fetch open positions and try to close them (placeholder)
	//t.requestInitialPositions()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
    for {
        select {
        case update := <-t.updates:
            t.adjustTargetPositionAccordingToAllocatedFundsUpdate(update)
        case sig := <-t.signalCh:
            t.handleSignal(sig)
        case ord := <-t.orderFeed:
            t.handleOrderUpdate(ord)
        case <-ticker.C:
            t.executeTradesToMakeActualTrackTarget()
        case <-t.ctx.Done():
            log.Printf("[Trader %s] Context done...", t.cfg.Symbol)
            t.closePositions()
            return
        }
    }
}

func (t *Trader) adjustTargetPositionAccordingToAllocatedFundsUpdate(update TradeCfg) {
	// adjust target position to maintain same target percentage after allocation change
	oldTargetPct := t.getTargetPositionPct()
	log.Printf("[Trader %s] AllocatedFunds updating from %v to %v", t.cfg.Symbol, t.cfg.AllocatedFunds, update.AllocatedFunds)
	t.cfg = update
	newTargetPct := t.getTargetPositionPct()
	targetPositionIncrease := (newTargetPct - oldTargetPct) * t.cfg.AllocatedFunds / 100.0
	t.targetPositionUSD += targetPositionIncrease
	log.Printf("[Trader %s] Target position increased by %v to a resulting value of %v", t.cfg.Symbol, targetPositionIncrease, t.targetPositionUSD)
}

// handleSignal executes buy/sell respecting rules on allocated funds and bounds 0..100
func (t *Trader) handleSignal(s Signal) {
	log.Printf("[Trader %s] Signal received: Percent=%v Type=%s", t.cfg.Symbol, s.Percent, s.Type)
	if s.Percent <= 0 {
		return
	}
	pct := s.Percent
	switch s.Type {
	case SignalBuy:
		// Buy percent pertains to allocated funds but cannot exceed 100% target
		t.targetPositionUSD += pct * t.cfg.AllocatedFunds / 100.0
		if t.targetPositionUSD > t.cfg.AllocatedFunds {
			t.targetPositionUSD = t.cfg.AllocatedFunds
		}
	case SignalSell:
		// Sell percent pertains to position if position > 100, else allocated funds percent
		targetPct := t.getTargetPositionPct()
		actualPct := t.getActualPositionPct()
		if actualPct > targetPct {
			pct *= actualPct / targetPct
		}
		if pct > targetPct {
			pct = targetPct
		}
		t.targetPositionUSD -= pct * t.cfg.AllocatedFunds / 100.0
	default:
		// hold not emitted
	}
}

func (t *Trader) closePositions() {
	log.Printf("[Trader %s] closing positions", t.cfg.Symbol)

}

func (t *Trader) executeTradesToMakeActualTrackTarget() {
	// to-do: add a tolerance = 2-4x the % fee for a single trade

	// if actual (minus gains or losses) - tolerance> target, sell

	// if actual (minus gains or losses) + tolerance < target, buy

	// else do nothing
}

func (t *Trader) executeBuyToTarget() {
	amountToBuy := t.targetPositionUSD - t.actualPositionUSDNoGainsOrLosses
	if err := t.submitBuyToCoinbase(amountToBuy); err != nil {
		log.Printf("failed to submit buy to coinbase: %v", err)
	}
}

func (t *Trader) executeSellToTarget() {
	amountToSell := t.actualPositionUSDNoGainsOrLosses - t.targetPositionUSD
	if err := t.submitSellToCoinbase(amountToSell); err != nil {
		log.Printf("failed to submit sell to coinbase: %v", err)
	}
}

func (t *Trader) submitBuyToCoinbase(amount float64) error{
	response, err := coinbaseClient.CreateOrder(t.ctx, t.cfg.Symbol, amount, true)
	if err != nil {
		log.Printf("failed to submit buy to coinbase: %v", err)
		return err
	}
	log.Printf("submitted buy to coinbase: %v", response)
	t.pendingOrder = getPendingOrder(response, SignalBuy, amount)
	return nil
}

func (t *Trader) submitSellToCoinbase(amount float64) error {
	response, err := coinbaseClient.CreateOrder(t.ctx, t.cfg.Symbol, amount, true)
	if err != nil {
		log.Printf("failed to submit buy to coinbase: %v", err)
		return err
	}
	log.Printf("submitted buy to coinbase: %v", response)
	t.pendingOrder = getPendingOrder(response, SignalBuy, amount)
	return nil

}

func (t *Trader) handleOrderUpdate(up OrderUpdate) {

}

func getPendingOrder(response CreateOrderResponse, orderType SignalType, amount float64) PendingOrder {
	return PendingOrder{
		OrderID: response.OrderID,
		SubmitTime: time.Now(),
		OrderType: orderType,
		AmountInUSD: amount,
	}
}

//  - i need to open a web socket connection in the trader manager to get order and position valuation updates.
//  - those updates have to go into the manager and be routed to the ui and the trader (we don't need to persist these yet).
//  - the trader will have to pull open positions in its cryptocurrency upon starting up. it will start by trying to close those positions.
//  - the trader needs to know the status of its orders so it can update its 'actual position' and 'actual position w/o gains or losses'.
//  - pretty sure we can remove profit loss channel from trader/manager in favor of an order results channel and position valuation channel.
//  - the trader has to let an order sit for some time before cancelling it and sending another,
//  - cancelling orders before the timeout if a reverse signal comes in is worth a shot. if it doesnt get cancelled in time, we'll get an update through the order fulfillment channel and the logic stays the same because all that does it update our actual position that the trader already has logic to adjust that to track the target position
//  - the trader has to assume the order will be filled and temporarily update the 'actual position w/o gains/losses' so it doesn't end up just submitting order after order while waiting for the order to be filled
//  - so the trader has to store its order details immediately upon submission and expect a certain time for an order confirmation to come through the corresponding channel from the manager and if that time arrives before the confirmation, you send a cancellation order then wipe the submission from memory
//  - all the existing coinbase interactions don't need jwts because they are public, but the ones pertaining to these details will need jwts, according to the coinbase docs: https://docs.cdp.coinbase.com/coinbase-app/advanced-trade-apis/websocket/websocket-overview
//  - so, order submission requests, order/position valuation web socket connection requests and subscription requests over that websocket will all need jwts inserted according to that documentation. please review it to make sure both the order submission requests and websocket requests have jwts inserted properly.
//  - please use the existing jwt creation functions to create the jwts

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
	// new: per-token signal channel
	signalCh chan Signal
	// track positions relative to allocated funds
	targetPositionPct float64 // 0..100
	actualPositionPct float64 // 0..100
	profitLossUpdates chan ProfitLossUpdate
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

	// Use HS256 here; change to jwt.SigningMethodRS256 and provide a private key if required.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
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
func NewTrader(cfg TradeCfg, ctx context.Context, cancel context.CancelFunc, updates chan TradeCfg, signalCh chan Signal, profitLossUpdates chan ProfitLossUpdate) *Trader {
	return &Trader{cfg: cfg, ctx: ctx, cancel: cancel, updates: updates, signalCh: signalCh, profitLossUpdates: profitLossUpdates}
}

// Run is the long‑running loop that talks to an exchange, processes signals, etc.
// It stops when ctx is cancelled.
func (t *Trader) Run() {
	log.Printf("[Trader %s] started – AllocatedFunds=%v", t.cfg.Symbol, t.cfg.AllocatedFunds)
	for {
		select {
		case update := <-t.updates:
			log.Printf("[Trader %s] AllocatedFunds updated – AllocatedFunds=%v", t.cfg.Symbol, update.AllocatedFunds)
			// adjust actual position to maintain same target percentage of new allocation
			t.cfg = update
			t.rebalanceToTarget()
		case sig := <-t.signalCh:
			t.handleSignal(sig)
		case <-t.ctx.Done():
			t.closePositions() // submit trades to close positions

			return
		default:
			t.doTradingLogic()
		}
	}
}

func (t *Trader) closePositions() {
	log.Printf("[Trader %s] closing positions", t.cfg.Symbol)
}

func (t *Trader) doTradingLogic() {
	// Idle work; primary actions are driven by signals and config updates
	time.Sleep(250 * time.Millisecond)
}

// handleSignal executes buy/sell respecting rules on allocated funds and bounds 0..100
func (t *Trader) handleSignal(s Signal) {
	if s.Percent <= 0 {
		return
	}
	pct := s.Percent
	switch s.Type {
	case SignalBuy:
		// Buy percent pertains to allocated funds but cannot exceed 100% target
		allowed := 100.0 - t.targetPositionPct
		if pct > allowed {
			pct = allowed
		}

		t.targetPositionPct += pct
		t.executeBuyToTarget()
	case SignalSell:
		// Sell percent pertains to position if position > 100, else allocated funds percent
		if t.actualPositionPct > t.targetPositionPct {
			pct *= t.actualPositionPct / t.targetPositionPct
		}
		if pct > t.targetPositionPct {
			pct = t.targetPositionPct
		}
		t.targetPositionPct -= pct
		t.executeSellToTarget()
	default:
		// hold not emitted
	}
}

func (t *Trader) rebalanceToTarget() {
	// If allocation changed, actual percentage shifts; bring actual to target
	if t.actualPositionPct < t.targetPositionPct {
		t.executeBuyToTarget()
	} else if t.actualPositionPct > t.targetPositionPct {
		t.executeSellToTarget()
	}
}

func (t *Trader) executeBuyToTarget() {
	if t.actualPositionPct >= t.targetPositionPct {
		return
	}
	delta := t.targetPositionPct - t.actualPositionPct
	// place buy order sized to 'delta' percent of allocated funds
	log.Printf("[Trader %s] BUY to target: delta=%.2f%% (actual=%.2f target=%.2f)", t.cfg.Symbol, delta, t.actualPositionPct, t.targetPositionPct)
	// simulate immediate fill
	t.actualPositionPct = t.targetPositionPct
}

func (t *Trader) executeSellToTarget() {
	if t.actualPositionPct <= t.targetPositionPct {
		return
	}
	delta := t.actualPositionPct - t.targetPositionPct
	// place sell order sized to 'delta' percent
	log.Printf("[Trader %s] SELL to target: delta=%.2f%% (actual=%.2f target=%.2f)", t.cfg.Symbol, delta, t.actualPositionPct, t.targetPositionPct)
	// simulate immediate fill
	t.actualPositionPct = t.targetPositionPct
}

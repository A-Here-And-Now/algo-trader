// traderManager.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// trader wraps everything we need to control a running trader.
type trader struct {
	cancel  context.CancelFunc // call to stop the goroutine
	done    chan struct{}      // closed when Run() exits
	cfg     TradeCfg           // keep the config for introspection / restart
	updates chan TradeCfg
}

type ProfitLossUpdate struct {
	Symbol     string
	ProfitLoss float64
}

// Manager holds a map of active traders keyed by a user‑supplied ID.
type Manager struct {
	mu                      sync.RWMutex       // protects the map
	traders                 map[string]*trader // ERC20 ticker -> trader
	ctx                     context.Context    // lifecycle context for manager
	wg                      sync.WaitGroup     // wait for all traders to stop
	Cfg                     ManagerCfg
	updates                 chan ManagerCfg
	profitLossUpdates       chan ProfitLossUpdate
	profitLossTotalForToday float64
	// added: signal engine and per-token channels
	engine      *SignalEngine
	signalChansForCalcEngine map[string]chan Signal
	priceFeedsForCalcEngine  map[string]chan Ticker
	candleFeedsForCalcEngine map[string]chan Candle
	stopPriceForCalcEngine   map[string]func()
	stopCandleForCalcEngine map[string]func()
	priceFeedsForFrontEnd  map[string]chan Ticker
	candleFeedsForFrontEnd map[string]chan Candle
	stopPriceForFrontEnd   map[string]func()
	stopCandleForFrontEnd map[string]func()
	priceHistory	map[string][]Ticker
	candleHistory	map[string][]Candle
}

type ManagerCfg struct {
	funds    float64
	maxPL    int64
	strategy Strategy
}

var wsUpgrader = websocket.Upgrader{
    // In dev you usually want to allow any origin.
    // In production lock this down to your domain.
    CheckOrigin: func(r *http.Request) bool { return true },
}


// NewManager builds an empty manager.
func NewManager(funds float64, maxPL int64, strategy Strategy) *Manager {
	updates := make(chan ManagerCfg)

	manager := Manager{
		traders: make(map[string]*trader),
		ctx:     context.Background(),
		Cfg: ManagerCfg{
			funds:    funds,
			maxPL:    maxPL,
			strategy: strategy,
		},
		updates:           updates,
		orderUpdates: make(chan ProfitLossUpdate, 256),
		signalChan:       make(chan Signal),
		priceFeedForCalcEngine:        make(chan Ticker),
		candleFeedForCalcEngine:       make(chan Candle),
		priceFeedForFrontEnd:        make(chan Ticker),
		candleFeedForFrontEnd:       make(chan Candle),
		priceHistory:	make(map[string][]Ticker, 50),
		candleHistory:	make(map[string][]Candle, 50),
	}

	// init and start signal engine
	manager.engine = NewSignalEngine(manager.ctx)
	manager.engine.Start()

	// start ticker and candle web socket for all tokens in tokens array 
	go func() {
		for {
			select {
			case manager.Cfg = <-updates:
			case profitLossUpdate := <-manager.profitLossUpdates:
				manager.profitLossTotalForToday += profitLossUpdate.ProfitLoss
			case <-manager.ctx.Done():
				return
			default:
				manager.checkProfitLossTotalForToday()
				time.Sleep(200 * time.Millisecond)
			}
		}
	}()

	return &manager
}

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
			var t tickerMsg
			if err := json.Unmarshal(raw, &t); err != nil {
				log.Printf("[WS] ticker unmarshal error: %v", err)
				continue
			}
			// non‑blocking send – drop if buffer full (price is high‑rate)
			for _, ticker := range t.Events[0].Tickers {
				m.writePrice(ticker)
			}

		case "candles":
			var c candleMsg
			if err := json.Unmarshal(raw, &c); err != nil {
				log.Printf("[WS] candle unmarshal error: %v", err)
				continue
			}
			// Coinbase returns an array of candles; we push each one.
			for _, candle := range c.Events[0].Candles {
				m.writeCandle(candle)
			}

		// Coinbase also sends keep‑alive messages like {"type":"heartbeat"}
		// – we just ignore them.
		default:
			// no‑op
		}
	}
}

func (m *Manager) writeCandle(candle Candle) {
	writeToChannelAndBufferLatest(m.candleFeedsForFrontEnd[candle.ProductID], candle)
	if _, ok := m.candleFeedsForFrontEnd[candle.ProductID]; ok {
		writeToChannelAndBufferLatest(m.candleFeedsForFrontEnd[candle.ProductID], candle)
	}
	if _, ok := m.candleFeedsForCalcEngine[candle.ProductID]; ok {
		writeToChannelAndBufferLatest(m.candleFeedsForCalcEngine[candle.ProductID], candle)
	}
	m.candleHistory[candle.ProductID] = append(m.candleHistory[candle.ProductID], candle)
	if len(m.candleHistory[candle.ProductID]) > 50 {
		m.candleHistory[candle.ProductID] = m.candleHistory[candle.ProductID][1:]
	}
}

func (m *Manager) writePrice(ticker Ticker) {
	writeToChannelAndBufferLatest(m.priceFeedsForFrontEnd[ticker.ProductID], ticker)
	if _, ok := m.priceFeedsForFrontEnd[ticker.ProductID]; ok {
		writeToChannelAndBufferLatest(m.priceFeedsForFrontEnd[ticker.ProductID], ticker)
	}
	if _, ok := m.priceFeedsForCalcEngine[ticker.ProductID]; ok {
		writeToChannelAndBufferLatest(m.priceFeedsForCalcEngine[ticker.ProductID], ticker)
	}
	m.priceHistory[ticker.ProductID] = append(m.priceHistory[ticker.ProductID], ticker)
	if len(m.priceHistory[ticker.ProductID]) > 50 {
		m.priceHistory[ticker.ProductID] = m.priceHistory[ticker.ProductID][1:]
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
	subPayload := []interface{}{
		map[string]interface{}{
			"type":        "subscribe",
			"product_ids": tokens,
			"channel":     "ticker_batch",
		},
		map[string]interface{}{
			"type":        "subscribe",
			"product_ids": tokens,
			"channel":     "candles",
		},
	}

	for _, p := range subPayload {
		for {
			err := conn.WriteJSON(p)
			if err == nil { break; }
		}
	}
}

// /ws?symbol=WETH
func (m *Manager) wsHandler(w http.ResponseWriter, r *http.Request) {
    // -------------------- 1️⃣ Parse query --------------------
    sym := r.URL.Query().Get("symbol")
    if sym == "" {
        http.Error(w, "symbol query param required", http.StatusBadRequest)
        return
    }

    // -------------------- 2️⃣ Upgrade HTTP → WS ---------------
    conn, err := wsUpgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Printf("[WS] upgrade error: %v", err)
        return
    }
    defer conn.Close()

	// create channels for the symbol
	m.priceFeedsForFrontEnd[sym] = make(chan Ticker, 1)
	m.candleFeedsForFrontEnd[sym] = make(chan Candle, 1)

	// We run a *single* goroutine that multiplexes both price and candle
    // channels onto the same WS connection.
    done := make(chan struct{}) // closed when the client disconnects

    go func() {
        // Ping the client every 30 s to keep the connection alive.
        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()
        for {
            select {
            case <-ticker.C:
                if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
                    // client likely closed, exit goroutine
                    close(done)
                    return
                }
            case <-done:
				close(m.priceFeedsForFrontEnd[sym])
				close(m.candleFeedsForFrontEnd[sym])
				delete(m.priceFeedsForFrontEnd, sym)
				delete(m.candleFeedsForFrontEnd, sym)
                return
            }
        }
    }()

    // Main pump – read from the two channels with a `select`.
    for {
        select {
        case p := <-m.priceFeedsForFrontEnd[sym]:
            if err := conn.WriteJSON(p); err != nil {
                log.Printf("[WS %s] write price error: %v", sym, err)
                close(done)
                return
            }

        case c := <-m.candleFeedsForFrontEnd[sym]:
            if err := conn.WriteJSON(c); err != nil {
                log.Printf("[WS %s] write candle error: %v", sym, err)
                close(done)
                return
            }

        case <-done:
            // client has closed the socket (or ping loop hit an error)
            return
        }
    }
}

// SetContext sets the lifecycle context used by the manager to gate actions
// during shutdown. If the context is canceled, certain operations (like
// reallocation) will be skipped.
func (m *Manager) SetContext(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ctx = ctx
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	traders := make([]*trader, 0, len(m.traders))
	for _, t := range m.traders {
		traders = append(traders, t)
	}
	// Clear map immediately
	m.traders = make(map[string]*trader)
	m.mu.Unlock()

	// Stop all traders concurrently
	for _, t := range traders {
		_ = m.Stop(t.cfg.Symbol)
	}

	// Optional: wait for all async Stop goroutines to finish
	doneCh := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(doneCh)
	}()

	// Optionally, enforce a global timeout for all stops
	select {
	case <-doneCh:
		log.Println("All traders stopped cleanly")
	case <-time.After(22 * time.Second):
		log.Println("Global timeout reached while waiting for traders to stop")
	}
}

// gets prices through websocket to coinbase and feed them to the signal engine

// Start creates a new trader goroutine.
// Returns an error if a trader with the same id is already running.
func (m *Manager) Start(tokenStr string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.traders[tokenStr]; exists {
		return fmt.Errorf("trader %q already running", tokenStr)
	}

	// Create a cancellable context for this trader.
	ctx, cancel := context.WithCancel(context.Background())

	// Channel that signals when the goroutine exits.
	done := make(chan struct{})

	var cfg TradeCfg
	cfg.Symbol = tokenStr

	// per-token channels
	m.priceFeedsForCalcEngine[tokenStr] = make(chan Ticker, 1)
	m.candleFeedsForCalcEngine[tokenStr] = make(chan Candle, 1)
	m.signalChansForCalcEngine[tokenStr] = make(chan Signal, 1)
	// register with signal engine
	m.engine.RegisterToken(tokenStr, m.priceFeedsForCalcEngine[tokenStr], m.candleFeedsForCalcEngine[tokenStr], m.signalChansForCalcEngine[tokenStr], m.priceHistory[tokenStr], m.candleHistory[tokenStr])

	// Build the trader and launch it in its own goroutine.
	updates := make(chan TradeCfg, 4)
	newTrader := NewTrader(cfg, ctx, cancel, updates, m.signalChansForCalcEngine[tokenStr], m.profitLossUpdates)
	go func() {
		// Ensure we close the done channel even on panic.
		defer close(done)
		newTrader.Run()
	}()

	// Store the handle.
	m.traders[tokenStr] = &trader{
		cancel:  cancel,
		done:    done,
		cfg:     cfg,
		updates: updates,
	}

	m.reallocateFunds()

	log.Printf("trader %q started (symbol=%s)", tokenStr, cfg.Symbol)
	return nil
}

// Stop cancels a running trader and waits (with a timeout) for it to finish.
// Returns an error if the trader does not exist.
func (m *Manager) Stop(token string) error {
	m.mu.Lock()
	t, exists := m.traders[token]
	if !exists {
		return fmt.Errorf("trader %q not found", token)
	}
	// Remove the entry immediately so a new Start() can happen
	delete(m.traders, token)
	close(m.priceFeedsForCalcEngine[token])
	close(m.candleFeedsForCalcEngine[token])
	close(m.signalChansForCalcEngine[token])

	// stop feeds and unregister
	delete(m.priceFeedsForCalcEngine, token)
	delete(m.candleFeedsForCalcEngine, token)
	delete(m.signalChansForCalcEngine, token)
	m.engine.UnregisterToken(token)
	m.mu.Unlock()

	// Cancel the trader context immediately
	t.cancel()

	// Wait asynchronously in a goroutine
	m.wg.Add(1)
	go func(tr *trader) {
		defer m.wg.Done()

		select {
		case <-tr.done:
			log.Printf("trader %q stopped cleanly", tr.cfg.Symbol)
			// Reallocate funds if manager context is still active
			if m.ctx.Err() == nil {
				m.mu.Lock()
				m.reallocateFunds()
				m.mu.Unlock()
			}
		case <-time.After(19 * time.Second):
			log.Printf("trader %q did not stop within timeout - need to pull active positions from exchange upon restart", tr.cfg.Symbol)
		}
	}(t)

	return nil
}

func (m *Manager) reallocateFunds() {
	numTraders := len(m.traders)
	if numTraders == 0 {
		return
	}
	allocated := m.Cfg.funds / float64(numTraders)
	for _, trader := range m.traders {
		writeToChannelAndBufferLatest(trader.updates, TradeCfg{
			Symbol:         trader.cfg.Symbol,
			AllocatedFunds: allocated,
			Strategy:       trader.cfg.Strategy,
		})
	}
}

func (m *Manager) updateStrategy(strategy Strategy) {
	numTraders := len(m.traders)
	if numTraders == 0 {
		return
	}
	m.Cfg.strategy = strategy
	for _, trader := range m.traders {
		writeToChannelAndBufferLatest(trader.updates, TradeCfg{
			Symbol:         trader.cfg.Symbol,
			AllocatedFunds: trader.cfg.AllocatedFunds,
			Strategy:       strategy,
		})
	}
}

// a function that will call stop all if the profit loss total for today is greater than the maxPL
func (m *Manager) checkProfitLossTotalForToday() {
	if m.profitLossTotalForToday > float64(m.Cfg.maxPL) {
		m.StopAll()
	}
}

func writeToChannelAndBufferLatest[T any](ch chan T, v T) {
    // First, try to send without blocking.
    select {
    case ch <- v:
        return // success – nothing else to do
    default:
        // Channel is full.  Drop the oldest entry (if any) and try again.
        select {
        case <-ch: // discard one element
        default: // nothing to discard – should be very rare
        }

        // Second attempt – this one should succeed because we just freed a slot.
        // If it still fails we just give up (same as the original code).
        select {
        case ch <- v:
        default:
        }
    }
}
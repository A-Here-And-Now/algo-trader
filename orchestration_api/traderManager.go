// traderManager.go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type ProfitLossUpdate struct {
	Symbol     string
	ProfitLoss float64
}

// Manager holds a map of active traders keyed by a user‑supplied ID.
type Manager struct {
	mu                      sync.RWMutex    // protects the map
	ctx                     context.Context // lifecycle context for manager
	wg                      sync.WaitGroup  // wait for all traders to stop
	Cfg                     ManagerCfg
	updates                 chan ManagerCfg
	profitLossUpdates       chan ProfitLossUpdate
	profitLossTotalForToday float64
	// added: signal engine and per-token channels
	engine               *SignalEngine
	traderResources      map[string]*TraderResource
	marketPriceResources map[string]*FrontEndResource
	subscriptionChannel  chan string
	frontendConnected    bool
	frontendMutex        sync.Mutex
}

func (m *Manager) safeAddMarketPriceResource(symbol string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.marketPriceResources[symbol] = NewFrontEndResource()
}

func (m *Manager) safeRemoveMarketPriceResource(symbol string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.marketPriceResources, symbol)
}

func (m *Manager) safeAddTraderResource(symbol string, cfg TradeCfg, done chan struct{}, cancel context.CancelFunc, updates chan TradeCfg) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.traderResources[symbol] = NewTraderResource(cfg, done, cancel, updates)
}

func (m *Manager) safeRemoveTraderResource(symbol string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.traderResources, symbol)
}

func (m *Manager) safeGetTraderResources() map[string]*TraderResource {
	m.mu.RLock()
	defer m.mu.RUnlock()
	resources := make(map[string]*TraderResource)
	for symbol, resource := range m.traderResources {
		resources[symbol] = resource
	}
	return resources
}

func (m *Manager) safeGetMarketPriceResources() map[string]*FrontEndResource {
	m.mu.RLock()
	defer m.mu.RUnlock()
	resources := make(map[string]*FrontEndResource)
	for symbol, resource := range m.marketPriceResources {
		resources[symbol] = resource
	}
	return resources
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
func NewManager(funds float64, maxPL int64, strategy Strategy, ctx context.Context) *Manager {
	updates := make(chan ManagerCfg)

	manager := Manager{
		Cfg: ManagerCfg{
			funds:    funds,
			maxPL:    maxPL,
			strategy: strategy,
		},
		ctx:                  ctx,
		updates:              updates,
		profitLossUpdates:    make(chan ProfitLossUpdate, 256),
		traderResources:      make(map[string]*TraderResource),
		marketPriceResources: make(map[string]*FrontEndResource),
		subscriptionChannel:  make(chan string),
		frontendConnected:    false,
		frontendMutex:        sync.Mutex{},
	}

	// init and start signal engine
	manager.engine = NewSignalEngine(manager.ctx)
	manager.engine.Start()

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

func (m *Manager) StopAll() {
	traders := m.safeGetTraderResources()

	m.mu.Lock()
	m.traderResources = make(map[string]*TraderResource)
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

// Start creates a new trader goroutine.
// Returns an error if a trader with the same id is already running.
func (m *Manager) Start(tokenStr string) error {
	if _, exists := m.safeGetTraderResources()[tokenStr]; exists {
		return fmt.Errorf("trader %q already running", tokenStr)
	}

	// Create a cancellable context for this trader.
	ctx, cancel := context.WithCancel(m.ctx)

	// Channel that signals when the goroutine exits.
	done := make(chan struct{})

	var cfg TradeCfg
	cfg.Symbol = tokenStr

	updates := make(chan TradeCfg, 4)

	// per-token channels
	m.safeAddTraderResource(tokenStr, cfg, done, cancel, updates)

	safeMarketPriceResources := m.safeGetMarketPriceResources()
	// register with signal engine
	m.engine.RegisterToken(tokenStr, m.traderResources[tokenStr], safeMarketPriceResources[tokenStr].priceHistory, safeMarketPriceResources[tokenStr].candleHistory)

	// Build the trader and launch it in its own goroutine.
	newTrader := NewTrader(cfg, ctx, cancel, updates, m.traderResources[tokenStr].signalChan, m.profitLossUpdates)
	go func() {
		// Ensure we close the done channel even on panic.
		defer close(done)
		newTrader.Run()
	}()

	m.reallocateFunds()

	log.Printf("trader %q started (symbol=%s)", tokenStr, cfg.Symbol)
	return nil
}

// Stop cancels a running trader and waits (with a timeout) for it to finish.
// Returns an error if the trader does not exist.
func (m *Manager) Stop(token string) error {
	t, exists := m.safeGetTraderResources()[token]
	if !exists {
		return fmt.Errorf("trader %q not found", token)
	}

	m.traderResources[token].Stop()
	m.safeRemoveTraderResource(token)
	m.engine.UnregisterToken(token)

	// Wait asynchronously in a goroutine
	m.wg.Add(1)
	go func(tr *TraderResource) {
		defer m.wg.Done()

		select {
		case <-tr.done:
			log.Printf("trader %q stopped cleanly", tr.cfg.Symbol)
			// Reallocate funds if manager context is still active
			if m.ctx.Err() == nil {
				m.reallocateFunds()
			}
		case <-time.After(19 * time.Second):
			log.Printf("trader %q did not stop within timeout - need to pull active positions from exchange upon restart", tr.cfg.Symbol)
		}
	}(t)

	return nil
}

func (m *Manager) reallocateFunds() {
	numTraders := len(m.safeGetTraderResources())
	if numTraders == 0 {
		return
	}
	allocated := m.Cfg.funds / float64(numTraders)
	for _, trader := range m.safeGetTraderResources() {
		writeToChannelAndBufferLatest(trader.updates, TradeCfg{
			Symbol:         trader.cfg.Symbol,
			AllocatedFunds: allocated,
			Strategy:       trader.cfg.Strategy,
		})
	}
}

func (m *Manager) updateStrategy(strategy Strategy) {
	numTraders := len(m.safeGetTraderResources())
	if numTraders == 0 {
		return
	}
	m.Cfg.strategy = strategy
	for _, trader := range m.safeGetTraderResources() {
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

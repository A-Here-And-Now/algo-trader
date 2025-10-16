// traderManager.go
package manager

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/channel_helper"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/entities/signaler"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/entities/trader"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	exchange "github.com/A-Here-And-Now/algo-trader/orchestration_api/exchange"
	coinbase_exchange "github.com/A-Here-And-Now/algo-trader/orchestration_api/exchange/coinbase"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
)

type Manager struct {
	mu                  sync.RWMutex
	ctx                 context.Context
	wg                  sync.WaitGroup
	Cfg                 ManagerCfg
	updates             chan ManagerCfg
	profitLossTotal     float64
	engine              *signaler.SignalEngine
	traderResources     map[string]*trader.TraderResource
	frontendConnected   bool
	frontendMutex       sync.Mutex
	tokenBalances       map[string]float64
	apiKey              string
	apiSecret           string
	tokens              []string
	exchange            exchange.IExchange
	signalEngineUpdates chan signaler.SignalEngineConfigUpdate
}

type ManagerCfg struct {
	funds            float64
	maxPL            int64
	tokenStrategies  map[string]enum.Strategy
	tokenCandleSizes map[string]enum.CandleSize
	tokenEnabled     map[string]bool
}

func (m *Manager) GetStrategy(token string) enum.Strategy {
	return m.Cfg.tokenStrategies[token]
}

func NewManager(funds float64, maxPL int64, startingStrategy enum.Strategy, startingCandleSize enum.CandleSize, ctx context.Context, apiKey string, apiSecret string, tokens []string) *Manager {
	updates := make(chan ManagerCfg)
	signalEngineUpdates := make(chan signaler.SignalEngineConfigUpdate, 10)

	manager := Manager{
		Cfg: ManagerCfg{
			funds:            funds,
			maxPL:            maxPL,
			tokenStrategies:  make(map[string]enum.Strategy),
			tokenCandleSizes: make(map[string]enum.CandleSize),
			tokenEnabled:     make(map[string]bool),
		},
		ctx:                 ctx,
		updates:             updates,
		traderResources:     make(map[string]*trader.TraderResource),
		frontendConnected:   false,
		frontendMutex:       sync.Mutex{},
		apiKey:              apiKey,
		apiSecret:           apiSecret,
		tokens:              tokens,
		exchange:            coinbase_exchange.NewCoinbaseExchange(ctx, apiKey, apiSecret, enum.CandleSize5m),
		signalEngineUpdates: signalEngineUpdates,
	}

	for _, token := range tokens {
		manager.Cfg.tokenStrategies[token] = startingStrategy
		manager.Cfg.tokenCandleSizes[token] = startingCandleSize
		manager.Cfg.tokenEnabled[token] = false
	}

	manager.engine = signaler.NewSignalEngine(manager.ctx, manager.exchange, signalEngineUpdates)

	go func() {
		for {
			select {
			case manager.Cfg = <-updates:
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

func (m *Manager) safeAddTraderResource(symbol string, cfg trader.TradeCfg, done chan struct{}, cancel context.CancelFunc, updates chan trader.TradeCfg) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tr := trader.NewTraderResource(cfg, done, cancel, updates)
	m.traderResources[symbol] = tr
}

func (m *Manager) safeRemoveTraderResource(symbol string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.traderResources, symbol)
}

func (m *Manager) safeGetTraderResources() map[string]*trader.TraderResource {
	m.mu.Lock()
	defer m.mu.Unlock()
	resources := make(map[string]*trader.TraderResource)
	for symbol, resource := range m.traderResources {
		resources[symbol] = resource
	}
	return resources
}

func (m *Manager) StopAll() {
	traders := m.safeGetTraderResources()

	m.mu.Lock()
	m.traderResources = make(map[string]*trader.TraderResource)
	m.mu.Unlock()

	for _, t := range traders {
		_ = m.Stop(t.Cfg.Symbol)
	}

	m.engine.Stop()
	close(m.signalEngineUpdates)

	doneCh := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(doneCh)
	}()

	select {
	case <-doneCh:
		log.Println("All traders stopped cleanly")
	case <-time.After(22 * time.Second):
		log.Println("Global timeout reached while waiting for traders to stop")
	}
}

func (m *Manager) Start(tokenStr string) error {
	if _, exists := m.safeGetTraderResources()[tokenStr]; exists {
		return fmt.Errorf("trader %q already running", tokenStr)
	}

	ctx, cancel := context.WithCancel(m.ctx)

	done := make(chan struct{})

	tradeCfg := trader.TradeCfg{
		Symbol:     tokenStr,
		Strategy:   m.Cfg.tokenStrategies[tokenStr],
		CandleSize: m.Cfg.tokenCandleSizes[tokenStr],
	}

	updates := make(chan trader.TradeCfg, 4)

	m.safeAddTraderResource(tokenStr, tradeCfg, done, cancel, updates)

	// Register with signal engine - note: engine will subscribe to exchange directly
	m.engine.RegisterToken(tokenStr, tradeCfg.Strategy, tradeCfg.CandleSize, m.traderResources[tokenStr].SignalChan)

	m.RefreshTokenBalances()

	// Create new trader - trader will subscribe to exchange directly for data feeds
	newTrader := trader.NewTrader(tradeCfg, ctx, cancel, updates, m.traderResources[tokenStr].SignalChan, m.tokenBalances[tokenStr], m.exchange)

	go func() {
		defer close(done)
		newTrader.Run()
	}()

	m.reallocateFunds()

	log.Printf("trader %q started (symbol=%s)", tokenStr, tradeCfg.Symbol)
	return nil
}

func (m *Manager) Stop(token string) error {
	t, exists := m.safeGetTraderResources()[token]
	if !exists {
		return fmt.Errorf("trader %q not found", token)
	}

	m.traderResources[token].Stop()
	m.safeRemoveTraderResource(token)
	m.engine.UnregisterToken(token)

	m.wg.Add(1)
	go func(tr *trader.TraderResource) {
		defer m.wg.Done()

		select {
		case <-tr.Done:
			log.Printf("trader %q stopped cleanly", tr.Cfg.Symbol)
			if m.ctx.Err() == nil {
				m.reallocateFunds()
			}
		case <-time.After(19 * time.Second):
			log.Printf("trader %q did not stop within timeout - need to pull active positions from exchange upon restart", tr.Cfg.Symbol)
		}
	}(t)

	return nil
}

func (m *Manager) RefreshTokenBalances() {
	balances, err := m.exchange.GetAllTokenBalances(m.ctx)
	if err != nil {
		panic(fmt.Sprintf("failed to get token balances: %v", err))
	}
	m.tokenBalances = balances
}

func (m *Manager) UpdateMaxPL(maxPL int64) {
	cfg := m.Cfg
	cfg.maxPL = maxPL
	m.updates <- cfg
}

func (m *Manager) UpdateStrategy(token string, strategy enum.Strategy) {
	m.Cfg.tokenStrategies[token] = strategy
	m.engine.UpdateStrategy(token, strategy)
}

func (m *Manager) GetAllPriceHistory() map[string][]models.Ticker {
	allPriceHistory := make(map[string][]models.Ticker)
	traders := m.safeGetTraderResources()
	for symbol := range traders {
		allPriceHistory[symbol] = m.exchange.GetPriceHistory(symbol)
	}
	return allPriceHistory
}

func (m *Manager) GetAllCandleHistory() map[string][]models.Candle {
	allCandleHistory := make(map[string][]models.Candle)
	traders := m.safeGetTraderResources()
	for symbol := range traders {
		candleHistory := m.exchange.GetCandleHistory(symbol)
		allCandleHistory[symbol] = candleHistory.Candles
	}
	return allCandleHistory
}

func (m *Manager) checkProfitLossTotalForToday() {
	if m.profitLossTotal > float64(m.Cfg.maxPL) {
		m.StopAll()
	}
}

func (m *Manager) reallocateFunds() {
	numTraders := len(m.safeGetTraderResources())
	if numTraders == 0 {
		return
	}
	allocated := m.Cfg.funds / float64(numTraders)
	for _, tr := range m.safeGetTraderResources() {
		channel_helper.WriteToChannelAndBufferLatest(tr.Updates, trader.TradeCfg{
			Symbol:         tr.Cfg.Symbol,
			AllocatedFunds: allocated,
			Strategy:       tr.Cfg.Strategy,
		})
	}
}

// traderManager.go
package manager

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/channel_helper"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/coinbase"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/entities/signaler"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/entities/trader"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
	"github.com/gorilla/websocket"
)

type Manager struct {
	mu           sync.RWMutex 
	ctx          context.Context
	wg           sync.WaitGroup
	Cfg          ManagerCfg
	updates      chan ManagerCfg
	marketDataWS *websocket.Conn
	userDataWS   *websocket.Conn
	profitLossTotal      float64
	engine               *signaler.SignalEngine
	traderResources      map[string]*trader.TraderResource
	marketPriceResources map[string]*models.FrontEndResource
	subscriptionChannel  chan string
	frontendConnected    bool
	frontendMutex        sync.Mutex
	client               *coinbase.CoinbaseClient
	tokenBalances        map[string]float64
	apiKey               string
	apiSecret            string
	tokens               []string
}

type ManagerCfg struct {
	funds    float64
	maxPL    int64
	strategy enum.Strategy
}

func (m *Manager) GetStrategy() enum.Strategy {
	return m.Cfg.strategy
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func NewManager(funds float64, maxPL int64, strategy enum.Strategy, ctx context.Context, apiKey string, apiSecret string, tokens []string) *Manager {
	updates := make(chan ManagerCfg)

	manager := Manager{
		Cfg: ManagerCfg{
			funds:    funds,
			maxPL:    maxPL,
			strategy: strategy,
		},
		ctx:     ctx,
		updates: updates,
		traderResources:      make(map[string]*trader.TraderResource),
		marketPriceResources: make(map[string]*models.FrontEndResource),
		subscriptionChannel:  make(chan string),
		frontendConnected:    false,
		frontendMutex:        sync.Mutex{},
		apiKey:               apiKey,
		apiSecret:            apiSecret,
		tokens:               tokens,
	}

	manager.client = coinbase.NewCoinbaseClient("https://api.coinbase.com", apiKey, apiSecret)

	manager.engine = signaler.NewSignalEngine(manager.ctx, strategy)

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

func (m *Manager) safeAddMarketPriceResource(symbol string, candleHistory26Days []models.Candle) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.marketPriceResources[symbol] = models.NewFrontEndResource(candleHistory26Days)
}

func (m *Manager) safeRemoveMarketPriceResource(symbol string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.marketPriceResources, symbol)
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
	m.mu.RLock()
	defer m.mu.RUnlock()
	resources := make(map[string]*trader.TraderResource)
	for symbol, resource := range m.traderResources {
		resources[symbol] = resource
	}
	return resources
}

func (m *Manager) safeGetMarketPriceResources() map[string]*models.FrontEndResource {
	m.mu.RLock()
	defer m.mu.RUnlock()
	resources := make(map[string]*models.FrontEndResource)
	for symbol, resource := range m.marketPriceResources {
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

	var cfg trader.TradeCfg
	cfg.Symbol = tokenStr

	updates := make(chan trader.TradeCfg, 4)

	m.safeAddTraderResource(tokenStr, cfg, done, cancel, updates)

	safeMarketPriceResources := m.safeGetMarketPriceResources()

	m.engine.RegisterToken(tokenStr, m.traderResources[tokenStr].PriceFeedToSignalEngine, m.traderResources[tokenStr].CandleFeedToSignalEngine,
		m.traderResources[tokenStr].SignalChan, safeMarketPriceResources[tokenStr].PriceHistory, safeMarketPriceResources[tokenStr].CandleHistory, safeMarketPriceResources[tokenStr].CandleHistory26Days)

	m.RefreshTokenBalances()

	newTrader := trader.NewTrader(cfg, ctx, cancel, updates, m.traderResources[tokenStr].SignalChan, m.traderResources[tokenStr].OrderFeed, m.tokenBalances[tokenStr])
	newTrader.CoinbaseClient = m.client

	go func() {
		defer close(done)
		newTrader.Run()
	}()

	m.reallocateFunds()

	log.Printf("trader %q started (symbol=%s)", tokenStr, cfg.Symbol)
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
	balances, err := m.client.GetAllTokenBalances(m.ctx)
	if err != nil {
		panic(fmt.Sprintf("failed to get token balances: %v", err))
	}
	m.tokenBalances = balances
}

func (m *Manager) UpdateMaxPL(maxPL int64) {
	m.updates <- ManagerCfg{
		maxPL:    maxPL,
		strategy: m.Cfg.strategy,
		funds:    m.Cfg.funds,
	}

}

func (m *Manager) UpdateStrategy(strategy enum.Strategy) {
	numTraders := len(m.safeGetTraderResources())
	if numTraders == 0 {
		return
	}
	m.Cfg.strategy = strategy
	m.engine.UpdateStrategy(strategy)
	for _, tr := range m.safeGetTraderResources() {
		tr.Cfg.Strategy = strategy
		channel_helper.WriteToChannelAndBufferLatest(tr.Updates, trader.TradeCfg{
			Symbol:         tr.Cfg.Symbol,
			AllocatedFunds: tr.Cfg.AllocatedFunds,
			Strategy:       strategy,
		})
	}
}

func (m *Manager) GetAllPriceHistory() map[string][]models.Ticker {
	allPriceHistory := make(map[string][]models.Ticker)
	safeMarketPriceResources := m.safeGetMarketPriceResources()
	for symbol, resource := range safeMarketPriceResources {
		allPriceHistory[symbol] = resource.PriceHistory
	}
	return allPriceHistory
}

func (m *Manager) GetAllCandleHistory() map[string][]models.Candle {
	allCandleHistory := make(map[string][]models.Candle)
	safeMarketPriceResources := m.safeGetMarketPriceResources()
	for symbol, resource := range safeMarketPriceResources {
		allCandleHistory[symbol] = resource.CandleHistory
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
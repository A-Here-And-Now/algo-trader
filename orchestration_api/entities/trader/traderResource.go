package trader

import (
	"context"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/coinbase"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
)

type TraderResource struct {
	SignalChan               chan enum.Signal
	PriceFeedToSignalEngine  chan models.Ticker
	PriceFeedToTrader        chan models.Ticker
	CandleFeedToSignalEngine chan models.Candle
	OrderFeed                chan coinbase.OrderUpdate
	Client                   *coinbase.CoinbaseClient
	StopPrice                func()
	StopCandle               func()
	Cancel                   context.CancelFunc // call to stop the goroutine
	Done                     chan struct{}      // closed when Run() exits
	Cfg                      TradeCfg           // keep the config for introspection / restart
	Updates                  chan TradeCfg
}

func NewTraderResource(cfg TradeCfg, done chan struct{}, cancel context.CancelFunc, updates chan TradeCfg) *TraderResource {
	return &TraderResource{
		SignalChan:               make(chan enum.Signal),
		PriceFeedToSignalEngine:  make(chan models.Ticker),
		PriceFeedToTrader:        make(chan models.Ticker),
		CandleFeedToSignalEngine: make(chan models.Candle),
		OrderFeed:                make(chan coinbase.OrderUpdate, 64),
		Cancel:                   cancel,
		Done:                     done,
		Cfg:                      cfg,
		Updates:                  updates,
	}
}

func (t *TraderResource) Stop() {
	t.Cancel()
	close(t.SignalChan)
	close(t.PriceFeedToSignalEngine)
	close(t.PriceFeedToTrader)
	close(t.CandleFeedToSignalEngine)
	close(t.OrderFeed)
}

package main

import "context"

type TraderResource struct {
	signalChan   chan Signal
	priceFeed    chan Ticker
	candleFeed   chan Candle
	orderFeed    chan OrderUpdate
	client       *CoinbaseClient
	stopPrice    func()
	stopCandle   func()
	cancel       context.CancelFunc // call to stop the goroutine
	done         chan struct{}      // closed when Run() exits
	cfg          TradeCfg           // keep the config for introspection / restart
	updates      chan TradeCfg
}

func NewTraderResource(cfg TradeCfg, done chan struct{}, cancel context.CancelFunc, updates chan TradeCfg) *TraderResource {
	return &TraderResource{
		signalChan:   make(chan Signal),
		priceFeed:    make(chan Ticker),
		candleFeed:   make(chan Candle),
		orderFeed:    make(chan OrderUpdate, 64),
		cancel:       cancel,
		done:         done,
		cfg:          cfg,
		updates:      updates,
	}
}

func (t *TraderResource) Stop() {
	t.cancel()
	close(t.signalChan)
	close(t.priceFeed)
	close(t.candleFeed)
	close(t.orderFeed)
}

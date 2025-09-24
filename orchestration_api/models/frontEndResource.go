package models

import (
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/coinbase"
)

type FrontEndResource struct {
	PriceFeed     chan Ticker
	CandleFeed    chan Candle
	OrderFeed     chan coinbase.OrderUpdate
	StopPrice     func()
	StopCandle    func()
	PriceHistory  []Ticker
	CandleHistory []Candle
}

func NewFrontEndResource() *FrontEndResource {
	return &FrontEndResource{
		PriceFeed:     make(chan Ticker),
		CandleFeed:    make(chan Candle),
		OrderFeed:     make(chan coinbase.OrderUpdate),
		PriceHistory:  make([]Ticker, 1),
		CandleHistory: make([]Candle, 1),
	}
}

func (t *FrontEndResource) Stop() {
	close(t.PriceFeed)
	close(t.CandleFeed)
	close(t.OrderFeed)
}

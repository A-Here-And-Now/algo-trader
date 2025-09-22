package main

import "time"

type SignalingResource struct {
	priceFeed     chan Ticker
	candleFeed    chan Candle
	signalCh      chan Signal
	priceHistory  []Ticker
	candleHistory []Candle
	lastSignalAt  time.Time
}

func NewSignalingResource(traderResource *TraderResource, priceHistory []Ticker, candleHistory []Candle) *SignalingResource {

	return &SignalingResource{
		priceFeed:     traderResource.priceFeed,
		candleFeed:    traderResource.candleFeed,
		signalCh:      traderResource.signalChan,
		priceHistory:  priceHistory,
		candleHistory: candleHistory,
		lastSignalAt:  time.Time{},
	}
}

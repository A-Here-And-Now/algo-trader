package main

type FrontEndResource struct {
	priceFeed     chan Ticker
	candleFeed    chan Candle
	orderFeed     chan OrderUpdate
	stopPrice     func()
	stopCandle    func()
	priceHistory  []Ticker
	candleHistory []Candle
}

func NewFrontEndResource() *FrontEndResource {
	return &FrontEndResource{
		priceFeed:     make(chan Ticker),
		candleFeed:    make(chan Candle),
		orderFeed:     make(chan OrderUpdate),
		priceHistory:  make([]Ticker, 1),
		candleHistory: make([]Candle, 1),
	}
}

func (t *FrontEndResource) Stop() {
	close(t.priceFeed)
	close(t.candleFeed)
	close(t.orderFeed)
}

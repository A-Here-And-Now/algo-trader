package models

type FrontEndResource struct {
	PriceFeed     chan Ticker
	CandleFeed    chan Candle
	OrderFeed     chan OrderUpdate
	StopPrice     func()
	StopCandle    func()
	PriceHistory  []Ticker
	CandleHistory []Candle
	CandleHistory26Days []Candle
}

func NewFrontEndResource(candleHistory26Days []Candle) *FrontEndResource {
	return &FrontEndResource{
		PriceFeed:     make(chan Ticker),
		CandleFeed:    make(chan Candle),
		OrderFeed:     make(chan OrderUpdate),
		PriceHistory:  make([]Ticker, 1),
		CandleHistory: make([]Candle, 1),
		CandleHistory26Days: candleHistory26Days,
	}
}

func (t *FrontEndResource) Stop() {
	close(t.PriceFeed)
	close(t.CandleFeed)
	close(t.OrderFeed)
}

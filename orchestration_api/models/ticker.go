package models

import (
	"time"
)

type Ticker struct {
	Symbol string    `json:"symbol"`
	Price  float64   `json:"price"`
	Time   time.Time `json:"time"`
}

type FrontEndTicker struct {
	Symbol string    `json:"symbol"`
	Price  float64   `json:"price"`
	Time   time.Time `json:"time"`
}

func GetFrontEndTicker(ticker Ticker) FrontEndTicker {
	return FrontEndTicker{
		Symbol: ticker.Symbol,
		Price:  ticker.Price,
		Time:   ticker.Time,
	}
}

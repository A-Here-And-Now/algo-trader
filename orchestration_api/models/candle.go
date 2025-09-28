package models

import (
	"time"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/coinbase"
)

type CandleMsg struct {
	Channel     string    `json:"channel"`
	ProductID   string    `json:"product_id"`
	ClientID    string    `json:"client_id"`
	Timestamp   time.Time `json:"timestamp"`
	SequenceNum int       `json:"sequence_num"`
	Events      []struct {
		Type    string   `json:"type"`
		Candles []Candle `json:"candles"`
	} `json:"events"`
}

type Candle struct {
	Start     string  `json:"start"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Open      float64 `json:"open"`
	Close     float64 `json:"close"`
	Volume    float64 `json:"volume"`
	ProductID string
}

func GetCandles(candles []coinbase.Candle, productID string) []Candle {
	candlesOut := make([]Candle, len(candles))
	for i, candle := range candles {
		candlesOut[i] = Candle{
			Start:     candle.Start,
			High:      candle.High,
			Low:       candle.Low,
			Open:      candle.Open,
			Close:     candle.Close,
			Volume:    candle.Volume,
			ProductID: productID,
		}
	}
	return candlesOut
}

type FrontEndCandle struct {
	Start  string  `json:"start"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Open   float64 `json:"open"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
	Symbol string  `json:"symbol"`
}

func (candle Candle) GetFrontEndCandle() FrontEndCandle {
	return FrontEndCandle{
		Start:  candle.Start,
		High:   candle.High,
		Low:    candle.Low,
		Open:   candle.Open,
		Close:  candle.Close,
		Volume: candle.Volume,
		Symbol: candle.ProductID,
	}
}

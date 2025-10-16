package models

import (
	"time"

	cb_models "github.com/A-Here-And-Now/algo-trader/orchestration_api/models/coinbase"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
)

type CandleMsg struct {
	Channel     string    `json:"channel"`
	ProductID   string    `json:"product_id"`
	ClientID    string    `json:"client_id"`
	Timestamp   time.Time `json:"timestamp"`
	SequenceNum int       `json:"sequence_num"`
	Events      []struct {
		Type    string           `json:"type"`
		Candles []CoinbaseCandle `json:"candles"`
	} `json:"events"`
}

type CoinbaseCandle struct {
	Start     string  `json:"start"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Open      float64 `json:"open"`
	Close     float64 `json:"close"`
	Volume    float64 `json:"volume"`
	ProductID string  `json:"product_id"`
}

func GetDomainCandlesFromHistoricalCandles(symbol string, histCandles []cb_models.CoinbaseHistoricalCandle) []Candle {
	candles := make([]Candle, 0)
	for _, c := range histCandles {
		candles = append(candles, Candle{
			Start:     GetTimeFromUnixTimestamp(c.Start),
			High:      c.High,
			Low:       c.Low,
			Open:      c.Open,
			Close:     c.Close,
			Volume:    c.Volume,
			ProductID: symbol,
		})
	}
	return candles
}

func (cc CoinbaseCandle) ToCandle() Candle {
	return Candle{
		Start:     GetTimeFromUnixTimestamp(cc.Start),
		High:      cc.High,
		Low:       cc.Low,
		Open:      cc.Open,
		Close:     cc.Close,
		Volume:    cc.Volume,
		ProductID: cc.ProductID,
	}
}

type Candle struct {
	Start     time.Time `json:"start"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Open      float64   `json:"open"`
	Close     float64   `json:"close"`
	Volume    float64   `json:"volume"`
	ProductID string    `json:"product_id"`
}

func (c *Candle) UpdateCandle(price float64, volume float64) {
	c.Close = price
	if price > c.High {
		c.High = price
	}
	if price < c.Low {
		c.Low = price
	}
	c.Volume = volume
}

func NewCandle(symbol string, previousCandleStart time.Time, candleSize enum.CandleSize, startPrice float64, volume float64) Candle {
	return Candle{
		Start:     previousCandleStart.Add(enum.GetTimeDurationFromCandleSize(candleSize)),
		High:      startPrice,
		Low:       startPrice,
		Open:      startPrice,
		Close:     startPrice,
		Volume:    volume,
		ProductID: symbol,
	}
}

type FrontEndCandle struct {
	Start  int64   `json:"start"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Open   float64 `json:"open"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
	Symbol string  `json:"symbol"`
}

func (candle Candle) GetFrontEndCandle() FrontEndCandle {
	return FrontEndCandle{
		Start:  candle.Start.Unix(),
		High:   candle.High,
		Low:    candle.Low,
		Open:   candle.Open,
		Close:  candle.Close,
		Volume: candle.Volume,
		Symbol: candle.ProductID,
	}
}

type HeikenAshiCandle struct {
	Start  time.Time
	High   float64
	Low    float64
	Open   float64
	Close  float64
	Volume float64
}

type RenkoCandle struct {
	Open  float64
	Close float64
}

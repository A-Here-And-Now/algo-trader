package signaler

import (
	"time"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
)

type SignalingResource struct {
	priceFeed     chan models.Ticker
	candleFeed    chan models.Candle
	signalCh      chan enum.Signal
	priceHistory  []models.Ticker
	candleHistory []models.Candle
	candleHistory26Days []models.Candle
	lastSignalAt  time.Time
	fiveMinuteCandleCounter int
}

func NewSignalingResource(priceFeed chan models.Ticker, candleFeed chan models.Candle, signalCh chan enum.Signal, priceHistory []models.Ticker, candleHistory []models.Candle, candleHistory26Days []models.Candle) *SignalingResource {

	return &SignalingResource{
		priceFeed:     priceFeed,
		candleFeed:    candleFeed,
		signalCh:      signalCh,
		priceHistory:  priceHistory,
		candleHistory: candleHistory,
		candleHistory26Days: candleHistory26Days,
		lastSignalAt:  time.Time{},
		fiveMinuteCandleCounter: 0,
	}
}

func (s *SignalingResource) Shift26DayCandleHistory(fiveMinuteCandles []models.Candle) {
	var newHistory models.Candle
	newHistory.Start = fiveMinuteCandles[0].Start
	newHistory.Open = fiveMinuteCandles[0].Open
	newHistory.Close = fiveMinuteCandles[len(fiveMinuteCandles)-1].Close
	newHistory.ProductID = fiveMinuteCandles[0].ProductID
	
	for _, candle := range fiveMinuteCandles {
		if candle.Low < newHistory.Low {
			newHistory.Low = candle.Low
		}
		if candle.High > newHistory.High {
			newHistory.High = candle.High
		}
		newHistory.Volume += candle.Volume
	}
	s.candleHistory26Days = append(s.candleHistory26Days, newHistory)
	s.candleHistory26Days = s.candleHistory26Days[1:]
}
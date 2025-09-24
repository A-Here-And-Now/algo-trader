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
	lastSignalAt  time.Time
}

func NewSignalingResource(priceFeed chan models.Ticker, candleFeed chan models.Candle, signalCh chan enum.Signal, priceHistory []models.Ticker, candleHistory []models.Candle) *SignalingResource {

	return &SignalingResource{
		priceFeed:     priceFeed,
		candleFeed:    candleFeed,
		signalCh:      signalCh,
		priceHistory:  priceHistory,
		candleHistory: candleHistory,
		lastSignalAt:  time.Time{},
	}
}

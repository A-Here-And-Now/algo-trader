package signaler

import (
	"sync"
	"time"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
)

type SignalingResource struct {
	mu 			  sync.RWMutex
	priceFeed     chan models.Ticker
	candleFeed    chan models.Candle
	signalCh      chan models.Signal
	priceHistory  []models.Ticker
	candleHistory []models.Candle
	candleHistory26Days []models.Candle
	lastSignalAt  time.Time
}

func NewSignalingResource(priceFeed chan models.Ticker, candleFeed chan models.Candle, signalCh chan models.Signal, priceHistory []models.Ticker, candleHistory []models.Candle, candleHistory26Days []models.Candle) *SignalingResource {

	return &SignalingResource{
		mu:            sync.RWMutex{},
		priceFeed:     priceFeed,
		candleFeed:    candleFeed,
		signalCh:      signalCh,
		priceHistory:  priceHistory,
		candleHistory: candleHistory,
		candleHistory26Days: candleHistory26Days,
		lastSignalAt:  time.Time{},
	}
}

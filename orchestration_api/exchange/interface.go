package exchange

import (
	"context"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
)

type IExchange interface {
	SubscribeToOrderUpdates(symbol string) (<-chan models.OrderUpdate, func(), error)
	SubscribeToTicker(symbol string) (<-chan models.Ticker, func(), error)
	SubscribeToCandle(symbol string) (<-chan models.Candle, func(), error)
	GetCandleHistory(symbol string) models.CandleHistory
	GetLongCandleHistory(symbol string) models.CandleHistory
	GetPriceHistory(symbol string) []models.Ticker
	UpdateInboundCandleSize(candleSize enum.CandleSize)
	StartNewTokenDataStream(symbol string, candleSize enum.CandleSize) error
	StopTokenDataStream(symbol string) error
	UpdateCandleSizeForSymbol(symbol string, candleSize enum.CandleSize) error
	StartOrderAndPositionValuationWebSocket(ctx context.Context, wsURL string)
	StartCoinbaseFeed(ctx context.Context, cbAdvUrl string)
}

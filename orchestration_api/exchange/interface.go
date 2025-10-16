package exchange

import (
	"context"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
	cb_models "github.com/A-Here-And-Now/algo-trader/orchestration_api/models/coinbase"
)

type IExchange interface {
	SubscribeToOrderUpdates(symbol string) (<-chan models.OrderUpdate, func())
	SubscribeToTicker(symbol string) (<-chan models.Ticker, func())
	SubscribeToCandle(symbol string) (<-chan models.Candle, func())
	GetCandleHistory(symbol string) models.CandleHistory
	GetLongCandleHistory(symbol string) models.CandleHistory
	GetPriceHistory(symbol string) []models.Ticker
	UpdateInboundCandleSize(candleSize enum.CandleSize)
	StartNewTokenDataStream(symbol string, candleSize enum.CandleSize) error
	StopTokenDataStream(symbol string) error
	UpdateCandleSizeForSymbol(symbol string, candleSize enum.CandleSize) error
	StartOrderAndPositionValuationWebSocket(ctx context.Context, wsURL string)
	StartCoinbaseFeed(ctx context.Context, cbAdvUrl string)
	// Coinbase API stuff (not sure what the better pattern than this is off the top of my head so this is fine for now)
	GetHistoricalCandles(ctx context.Context, productID string, candleSize enum.CandleSize) (cb_models.CandlesResponse, error)
	ListAccounts(ctx context.Context) (cb_models.AccountsListResponse, error)
	GetAllTokenBalances(ctx context.Context) (map[string]float64, error)
	ListOrders(ctx context.Context, productID string, limit int) (cb_models.ListOrdersResponse, error)
	CreateOrder(ctx context.Context, productID string, amountOfUSD float64, isBuy bool) (cb_models.CreateOrderResponse, error)
	SellTokens(ctx context.Context, productID string, amountOfUSD float64) (cb_models.CreateOrderResponse, error)
	EditOrder(ctx context.Context, body []byte) (cb_models.EditOrderResponse, error)
	CancelOrders(ctx context.Context, orderID string) error
}

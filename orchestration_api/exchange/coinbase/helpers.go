package coinbase

import (
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/coinbase"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
)

// GetOrderUpdate converts a Coinbase Order to an OrderUpdate
func GetOrderUpdate(o coinbase.Order) models.OrderUpdate {
	return models.OrderUpdate{
		Channel:       "user",
		ProductID:     o.ProductID,
		OrderID:       o.OrderID,
		Status:        o.Status,
		FilledQty:     o.CumulativeQuantity,
		FilledValue:   o.FilledValue,
		CompletionPct: o.CompletionPct,
		Leaves:        o.Leaves,
		Price:         o.AvgPrice,
		Side:          o.OrderSide,
		Ts:            models.GetTimeFromUnixTimestamp(o.CreationTime),
	}
}

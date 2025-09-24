package trader

import (
	"log"
	"time"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/coinbase"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
)

func (t *Trader) getPendingOrderFromResponse(response coinbase.CreateOrderResponse, orderType enum.SignalType, amount float64) models.PendingOrder {
	return models.PendingOrder{
		OrderID:                          response.OrderID,
		SubmitTime:                       time.Now(),
		OrderType:                        orderType,
		OriginalAmountInUSD:              amount,
		CurrentAmountLeftToBeFilledInUSD: amount,
		AlreadyFilledInUSD:               0,
		OriginalAmountInTokens:           amount / t.currentPriceUSDPerToken,
		AlreadyFilledInTokens:            0,
	}
}

func (t *Trader) hasPendingOrder() bool {
	if t.pendingOrder != nil {
		if t.pendingOrder.CurrentAmountLeftToBeFilledInUSD > 0 {
			return true
		} else {
			log.Printf("pending order amount is 0, clearing pending order")
			t.clearPendingOrder()
		}
	}
	return false
}

func (t *Trader) clearPendingOrder() {
	t.pendingOrder = nil
}

func (t *Trader) setPendingOrder(order models.PendingOrder) {
	t.pendingOrder = &order
}

func (t *Trader) updatePendingOrderBalance(amount float64) {
	t.pendingOrder.CurrentAmountLeftToBeFilledInUSD -= amount
}
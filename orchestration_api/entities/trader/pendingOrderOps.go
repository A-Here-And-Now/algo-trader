package trader

import (
	"log"
	"time"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
	enum "github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	cb_models "github.com/A-Here-And-Now/algo-trader/orchestration_api/models/coinbase"
)

func (t *Trader) getPendingOrderFromResponse(response cb_models.CreateOrderResponse, orderType enum.SignalType, amount float64) models.PendingOrder {
	return models.PendingOrder{
		OrderID:                          response.OrderID,
		SubmitTime:                       time.Now(),
		OrderType:                        orderType,
		OriginalAmountInUSD:              amount,
		CurrentAmountLeftToBeFilledInUSD: amount,
		AlreadyFilledInUSD:               0,
		OriginalAmountInTokens:           amount / t.state.CurrentPriceUSDPerToken,
		AlreadyFilledInTokens:            0,
	}
}

func (t *Trader) hasPendingOrder() bool {
	if t.state.PendingOrder != nil {
		if t.state.PendingOrder.CurrentAmountLeftToBeFilledInUSD > 0 {
			return true
		} else {
			log.Printf("pending order amount is 0, clearing pending order")
			t.clearPendingOrder()
		}
	}
	return false
}

func (t *Trader) clearPendingOrder() {
	t.state.PendingOrder = nil
}

func (t *Trader) setPendingOrder(order models.PendingOrder) {
	t.state.PendingOrder = &order
}

func (t *Trader) updatePendingOrderBalances(currentAmountLeftToBeFilledInUSD float64, alreadyFilledUSD float64, alreadyFilledTokens float64) {
	t.state.PendingOrder.CurrentAmountLeftToBeFilledInUSD = currentAmountLeftToBeFilledInUSD
	t.state.PendingOrder.AlreadyFilledInUSD = alreadyFilledUSD
	t.state.PendingOrder.AlreadyFilledInTokens = alreadyFilledTokens
}
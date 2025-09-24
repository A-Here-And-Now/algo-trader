package main

import (
	"log"
	"time"
)

type PendingOrder struct {
	OrderID                          string
	SubmitTime                       time.Time
	OrderType                        SignalType
	OriginalAmountInUSD              float64
	CurrentAmountLeftToBeFilledInUSD float64
	AlreadyFilledInUSD               float64
	OriginalAmountInTokens           float64
	AlreadyFilledInTokens            float64
}

func (t *Trader) getPendingOrderFromResponse(response CreateOrderResponse, orderType SignalType, amount float64) *PendingOrder {
	return &PendingOrder{
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

func (t *Trader) setPendingOrder(order PendingOrder) {
	t.pendingOrder = &order
}

func (t *Trader) updatePendingOrderBalance(amount float64) {
	t.pendingOrder.CurrentAmountLeftToBeFilledInUSD -= amount
}

package models

import (
	"time"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
)

type PendingOrder struct {
	OrderID                          string
	SubmitTime                       time.Time
	OrderType                        enum.SignalType
	OriginalAmountInUSD              float64
	CurrentAmountLeftToBeFilledInUSD float64
	AlreadyFilledInUSD               float64
	OriginalAmountInTokens           float64
	AlreadyFilledInTokens            float64
}


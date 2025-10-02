package models

import (
	"time"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
)

type Signal struct {
	Symbol  string
	Type    enum.SignalType
	Percent float64
	Time    time.Time
	TakeProfit float64
	StopLoss float64
	Price float64
}

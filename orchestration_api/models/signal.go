package models

import (
	"time"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
)

type Signal struct {
	Symbol  string
	Type    enum.SignalType
	Percent float64 // 0-100 meaning percent of allocated funds or position per rules
	Time    time.Time
}

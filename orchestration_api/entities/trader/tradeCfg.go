package trader

import "github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"

type TradeCfg struct {
	Symbol         string        `json:"symbol"`   // e.g. "BTCUSD"
	AllocatedFunds float64       `json:"size"`     // position size
	Strategy       enum.Strategy `json:"strategy"` // trading strategy
}
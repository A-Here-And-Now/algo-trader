package strategy_helper

import (
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	models "github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
)

type PositionState struct {
	InPosition bool
	Side       enum.SignalType
	EntryPrice float64
	TakeProfit float64
	StopLoss   float64
}

// Holds the map and the common ConfirmSignalDelivered implementation.
type PositionHolder struct {
	State map[string]*PositionState
}

func NewPositionHolder() *PositionHolder {
	return &PositionHolder{State: make(map[string]*PositionState)}
}

func (h *PositionHolder) ConfirmSignalDelivered(symbol string, signal models.Signal) {
	if _, ok := h.State[symbol]; !ok {
		h.State[symbol] = &PositionState{}
	}
	h.State[symbol].Side = signal.Type
	h.State[symbol].EntryPrice = signal.Price
	h.State[symbol].InPosition = signal.Type == enum.SignalBuy
	if h.State[symbol].InPosition {
		h.State[symbol].TakeProfit = signal.TakeProfit
		h.State[symbol].StopLoss = signal.StopLoss
	} else {
		h.State[symbol].TakeProfit = 0
		h.State[symbol].StopLoss = 0
	}
}
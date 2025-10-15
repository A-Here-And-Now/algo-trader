package strategy_helper

import (
	"strconv"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	models "github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
)

type PositionState struct {
	InPosition            	  bool
	Side                  	  enum.SignalType
	EntryPrice            	  float64
	TakeProfit            	  float64
	StopLoss              	  float64
	TrailingStop              float64
	LastTrailingStopPrice     float64
	PositionIncreaseThreshold float64
}

// Holds the map and the common ConfirmSignalDelivered implementation.
type PositionHolder struct {
	State map[string]*PositionState
}

func NewPositionHolder() *PositionHolder {
	return &PositionHolder{State: make(map[string]*PositionState)}
}

func NewInPositionState(signal models.Signal) *PositionState {
	return &PositionState{
		Side: signal.Type,
		EntryPrice: signal.Price,
		InPosition: signal.Type == enum.SignalBuy,
		TakeProfit: signal.TakeProfit,
		StopLoss: signal.StopLoss,
		TrailingStop: signal.TrailingStop,
		PositionIncreaseThreshold: signal.PositionIncreaseThreshold,
		LastTrailingStopPrice: signal.LastTrailingStopPrice,
	}
}

func NewPositionState(symbol string) *PositionState {
	return &PositionState{ }
}

func (h *PositionHolder) ConfirmSignalDelivered(symbol string, signal models.Signal) {
	if _, ok := h.State[symbol]; !ok {
		h.State[symbol] = &PositionState{}
	}
	h.State[symbol].Side = signal.Type
	h.State[symbol].EntryPrice = signal.Price
	h.State[symbol].InPosition = signal.Type == enum.SignalBuy
	if h.State[symbol].InPosition {
		h.State[symbol] = NewInPositionState(signal)
	} else {
		h.State[symbol] = NewPositionState(symbol)
	}
}

func (h *PositionHolder) UpdateTrailingStop(symbol string, ticker models.Ticker) {
	if s, ok := h.State[symbol]; ok && s.InPosition && s.TrailingStop != 0 {
		currentPrice, _ := strconv.ParseFloat(ticker.Price, 64)
		if currentPrice > s.LastTrailingStopPrice {
			s.TrailingStop += currentPrice - s.LastTrailingStopPrice
			s.LastTrailingStopPrice = currentPrice
		}
	}
}

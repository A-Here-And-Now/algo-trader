package strategies

import (
	"time"

	helper "github.com/A-Here-And-Now/algo-trader/orchestration_api/entities/signaler/strategy_helper"
	enum "github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	models "github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
	talib "github.com/markcheno/go-talib"
)

type GroverLlorensActivatorStrategy struct {
	*helper.PositionHolder
	Length     int     // ATR period for base calculation
	Mult       float64 // multiplier for ATR in ts update
	TsAtrMult  float64 // trailing stop multiplier
	TpAtrMult  float64 // take profit multiplier
}

func (s *GroverLlorensActivatorStrategy) CalculateSignal(symbol string, priceStore helper.IPriceActionStore) models.Signal {
	hist := priceStore.GetFullMergedCandleHistory(symbol)
	closes := hist.GetCloses()
	highs := hist.GetHighs()
	lows := hist.GetLows()

	n := len(closes)
	if n < s.Length+2 {
		return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
	}

	atr := talib.Atr(highs, lows, closes, s.Length)

	// --- Initialize slices ---
	ts := make([]float64, n)
	diff := make([]float64, n)
	up := make([]bool, n)
	dn := make([]bool, n)
	lastCrossIndex := 0

	// First bar: initialize ts
	ts[0] = closes[0]

	for i := 1; i < n; i++ {
		// Compute diff relative to previous ts
		diff[i] = closes[i] - ts[i-1]

		// Detect crossovers
		up[i] = diff[i-1] <= 0 && diff[i] > 0
		dn[i] = diff[i-1] >= 0 && diff[i] < 0

		// Determine ts[i] based on crossover
		if up[i] {
			ts[i] = ts[i-1] - atr[i]*s.Mult
			lastCrossIndex = i
		} else if dn[i] {
			ts[i] = ts[i-1] + atr[i]*s.Mult
			lastCrossIndex = i
		} else {
			barsSince := i - lastCrossIndex
			val := atr[lastCrossIndex] / float64(s.Length)
			ts[i] = ts[i-1] + helper.GetSign(diff[i])*val*float64(barsSince)
		}
	}

	// --- Current bar ---
	closeCurr := closes[n-1]
	state := s.PositionHolder.State[symbol]
	inPosition := state.InPosition

	// --- Trailing stop / take profit ---
	atrVal := atr[n-1]
	trailingStop := closeCurr - s.TsAtrMult*atrVal
	takeProfit := closeCurr + s.TpAtrMult*atrVal

	// --- Generate signals based on last crossover ---
	if up[n-1] && !inPosition {
		return models.Signal{
			Symbol:       symbol,
			Type:         enum.SignalBuy,
			Percent:      100,
			Time:         time.Now(),
			TrailingStop: trailingStop,
			TakeProfit:   takeProfit,
			Price:        closeCurr,
		}
	}

	// Exit conditions
	if inPosition {
		isReachedTP := closeCurr >= state.TakeProfit
		isReachedStop := closeCurr <= state.TrailingStop
		isBearishFlip := dn[n-1]

		if isReachedTP || isReachedStop || isBearishFlip {
			return models.Signal{
				Symbol:  symbol,
				Type:    enum.SignalSell,
				Percent: 100,
				Time:    time.Now(),
				Price:   closeCurr,
			}
		}
	}

	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

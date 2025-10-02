package strategies

import (
	"math"
	"time"

	helper "github.com/A-Here-And-Now/algo-trader/orchestration_api/entities/signaler/strategy_helper"
	enum "github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	models "github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
	talib "github.com/markcheno/go-talib"
)

type HeikenAshiStrategy struct {
	*helper.PositionHolder
	AtrPeriod         int
	AtrLineMultiplier float64
	TpATRMultiplier   float64
	SlATRMultiplier   float64
	NumEmaPeriods     int
}

func (s *HeikenAshiStrategy) CalculateSignal(symbol string, priceStore helper.IPriceActionStore) models.Signal {
	hist := priceStore.GetCandleHistory(symbol)
	haCandles := hist.GetHeikenAshiCandleHistory()
	haCloses := haCandles.GetHeikenAshiCloses()
	haHighs := haCandles.GetHeikenAshiHighs()
	haLows := haCandles.GetHeikenAshiLows()
	lows := hist.GetLows()
	highs := hist.GetHighs()
	closes := hist.GetCloses()
	state := s.PositionHolder.State[symbol]

	n := len(haCloses)
	i := n - 1
	if n < s.AtrPeriod+2 || n < s.NumEmaPeriods {
		return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
	}

	// --- ATR CALCULATION (from TA-Lib) ---
	atr := talib.Atr(haHighs, haLows, haCloses, s.AtrPeriod)

	// --- EMA CALCULATION (from TA-Lib) ---
	ema := talib.Ema(haCloses, s.NumEmaPeriods)

	// --- ATR TRAILING STOP LINE ---
	// This is a trailing stop line, not a trailing stop price.
	// They are not the same thing, nor are they functionally related.
	xATRTrailingStopLine := make([]float64, n)
	for j := 1; j < n; j++ {
		nLoss := s.AtrLineMultiplier * atr[j]
		prev := xATRTrailingStopLine[j-1]

		if haCloses[j] > prev && haCloses[j-1] > prev {
			xATRTrailingStopLine[j] = math.Max(prev, haCloses[j]-nLoss)
		} else if haCloses[j] < prev && haCloses[j-1] < prev {
			xATRTrailingStopLine[j] = math.Min(prev, haCloses[j]+nLoss)
		} else if haCloses[j] > prev {
			xATRTrailingStopLine[j] = haCloses[j] - nLoss
		} else {
			xATRTrailingStopLine[j] = haCloses[j] + nLoss
		}
	}

	// --- SIGNAL GENERATION ---
	above := haCloses[i-1] <= xATRTrailingStopLine[i-1] && haCloses[i] > xATRTrailingStopLine[i]
	below := haCloses[i-1] >= xATRTrailingStopLine[i-1] && haCloses[i] < xATRTrailingStopLine[i]

	// Combine ATR stop and EMA trend
	inUpTrend := haCloses[i] > ema[i]
	inDownTrend := haCloses[i] < ema[i]

	buySignal := haCloses[i] > xATRTrailingStopLine[i] && above && inUpTrend
	sellSignal := haCloses[i] < xATRTrailingStopLine[i] && below && inDownTrend

	// --- ENTRY / EXIT CONDITIONS ---
	longEntry := buySignal && !state.InPosition
	stopLossHit := state.InPosition && lows[i] <= state.StopLoss
	atrExit := sellSignal && state.InPosition
	takeProfitHit := highs[i] >= state.TakeProfit && state.InPosition
	longExit := atrExit || stopLossHit || takeProfitHit

	// --- UPDATE STATE & RETURN SIGNAL ---
	if longEntry {
		// --- STOP LOSS & TAKE PROFIT CALCULATION ---
		stopLoss := closes[i] - (s.SlATRMultiplier * atr[i])
		takeProfit := closes[i] + (s.TpATRMultiplier * atr[i])
		return models.Signal{
			Symbol:     symbol,
			Type:       enum.SignalBuy,
			Percent:    100,
			Time:       time.Now(),
			StopLoss:   stopLoss,
			TakeProfit: takeProfit,
			Price:      closes[i],
		}
	}
	if longExit {
		return models.Signal{
			Symbol:     symbol,
			Type:       enum.SignalSell,
			Percent:    100,
			Time:       time.Now(),
			StopLoss:   0,
			TakeProfit: 0,
			Price:      closes[i],
		}
	}

	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

package strategies

import (
	"time"

	helper "github.com/A-Here-And-Now/algo-trader/orchestration_api/entities/signaler/strategy_helper"
	enum "github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	models "github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
	talib "github.com/markcheno/go-talib"
)

type TurtleTraderStrategy struct {
	*helper.PositionHolder
	NumberOfPeriods      int
	PredictionUnit       string  // "atr" or "percent"
	PredictionMultiplier float64 // multiplier
	UsePullbackFilter    bool
}

func (s *TurtleTraderStrategy) CalculateSignal(symbol string, priceStore helper.IPriceActionStore) models.Signal {
	hist := priceStore.GetFullMergedCandleHistory(symbol)
	highs := hist.GetHighs()
	lows := hist.GetLows()
	closes := hist.GetCloses()
	n := len(closes)
	if n < s.NumberOfPeriods+4 {
		return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
	}

	// --- Donchian channel ---
	hb := make([]float64, n)
	lb := make([]float64, n)
	med := make([]float64, n)
	dist := make([]float64, n)
	for i := s.NumberOfPeriods - 1; i < n; i++ {
		highMax := highs[i-s.NumberOfPeriods+1]
		lowMin := lows[i-s.NumberOfPeriods+1]
		for j := i - s.NumberOfPeriods + 1; j <= i; j++ {
			if highs[j] > highMax {
				highMax = highs[j]
			}
			if lows[j] < lowMin {
				lowMin = lows[j]
			}
		}
		hb[i] = highMax
		lb[i] = lowMin
		dist[i] = hb[i] - lb[i]
		med[i] = (hb[i] + lb[i]) / 2
	}

	// --- Fibonacci levels ---
	hf := make([]float64, n)
	chf := make([]float64, n)
	clf := make([]float64, n)
	lf := make([]float64, n)
	for i := s.NumberOfPeriods - 1; i < n; i++ {
		hf[i] = hb[i] - dist[i]*0.236
		chf[i] = hb[i] - dist[i]*0.382
		clf[i] = hb[i] - dist[i]*0.618
		lf[i] = hb[i] - dist[i]*0.764
	}

	// --- Horizontal breakout detection ---
	leh := make([]float64, n)
	lel := make([]float64, n)
	hbtrue := make([]bool, n)
	for i := 3; i < n; i++ {
		evpup := highs[i] > hb[i-1]
		evhbstart := hb[i-3] == hb[i-2] && hb[i-2] == hb[i-1] && evpup

		evpdown := lows[i] < lb[i-1]
		evlbstart := lb[i-3] == lb[i-2] && lb[i-2] == lb[i-1] && evpdown

		evhb := evhbstart || highs[i-1] == hb[i]
		leh[i] = leh[i-1]
		if evhb {
			leh[i] = hf[i]
		}

		evlb := evlbstart || lows[i-1] == lb[i]
		lel[i] = lel[i-1]
		if evlb {
			lel[i] = lf[i]
		}

		if evhb {
			hbtrue[i] = true
		} else if evlb {
			hbtrue[i] = false
		} else {
			hbtrue[i] = hbtrue[i-1]
		}
	}

	// --- ATR for prediction levels ---
	atr := talib.Atr(highs, lows, closes, s.NumberOfPeriods)
	pred := make([]float64, n)
	for i := s.NumberOfPeriods - 1; i < n; i++ {
		switch s.PredictionUnit {
		case "atr":
			pred[i] = s.PredictionMultiplier * atr[i]
		case "percent":
			pred[i] = s.PredictionMultiplier * closes[i] / 100.0
		}
	}

	// --- Trend detection ---
	i := n - 1
	inUpTrend := hbtrue[i] && closes[i] > hf[i]
	inDownTrend := !hbtrue[i] && closes[i] < lf[i]

	// --- Event detection for entry signals ---
	evpup := highs[i] > hb[i-1]
	evpdown := lows[i] < lb[i-1]

	// --- Pullback filter for signals ---
	buySignal := inUpTrend && evpup && (!s.UsePullbackFilter || closes[i] > leh[i])      // only buy above last high pullback
	sellSignal := inDownTrend && evpdown && (!s.UsePullbackFilter || closes[i] < lel[i]) // only sell below last low pullback

	// --- Prediction levels for stop-loss / take-profit ---
	var trailingStop, takeProfit, positionIncreaseThreshold float64
	if buySignal {
		trailingStop = closes[i] - (0.5 * pred[i])
		takeProfit = closes[i] + pred[i]
		positionIncreaseThreshold = closes[i] + (0.20 * pred[i])
	}

	// --- Return signal ---
	if !s.PositionHolder.State[symbol].InPosition && buySignal {
		return models.Signal{
			Symbol:       symbol,
			Type:         enum.SignalBuy,
			Percent:      50,
			Time:         time.Now(),
			TrailingStop: trailingStop,
			TakeProfit:   takeProfit,
			Price:        closes[i],
			PositionIncreaseThreshold: positionIncreaseThreshold,
			LastTrailingStopPrice:     closes[i],
		}
	} else if s.PositionHolder.State[symbol].InPosition {
		isReachedTakeProfit := closes[i] >= s.PositionHolder.State[symbol].TakeProfit
		isReachedTrailingStop := closes[i] <= s.PositionHolder.State[symbol].TrailingStop
		isReachedPositionIncreaseThreshold := closes[i] >= s.PositionHolder.State[symbol].PositionIncreaseThreshold
		if isReachedTakeProfit || isReachedTrailingStop {
			return models.Signal{
				Symbol:  symbol,
				Type:    enum.SignalSell,
				Percent: 100,
				Time:    time.Now(),
				Price:   closes[i],
			}
		} else if isReachedPositionIncreaseThreshold {
			return models.Signal{
				Symbol:                    symbol,
				Type:                      enum.SignalBuy,
				Percent:                   12.5,
				Time:                      time.Now(),
				PositionIncreaseThreshold: positionIncreaseThreshold,
				TrailingStop:              s.PositionHolder.State[symbol].TrailingStop,
				TakeProfit:                s.PositionHolder.State[symbol].TakeProfit,
				Price:                     closes[i],
				LastTrailingStopPrice:     s.PositionHolder.State[symbol].LastTrailingStopPrice,
			}
		}
	} else if sellSignal {
		return models.Signal{
			Symbol:       symbol,
			Type:         enum.SignalSell,
			Percent:      100,
			Time:         time.Now(),
			Price:        closes[i],
		}
	}

	return models.Signal{
		Symbol:  symbol,
		Type:    enum.SignalHold,
		Percent: 0,
		Time:    time.Now(),
	}
}

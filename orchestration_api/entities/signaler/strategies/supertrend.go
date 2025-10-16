package strategies

import (
	"math"
	"time"

	helper "github.com/A-Here-And-Now/algo-trader/orchestration_api/entities/signaler/strategy_helper"
	enum "github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	models "github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
	talib "github.com/markcheno/go-talib"
	exchange "github.com/A-Here-And-Now/algo-trader/orchestration_api/exchange"
)

type SupertrendStrategy struct {
	*helper.PositionHolder
	AtrPeriod   int
	Factor      float64
	UseVolFilt  bool
	VolLen      int
	TsAtrMult   float64
	TpAtrMult   float64
}

func (s *SupertrendStrategy) CalculateSignal(symbol string, exchange exchange.IExchange) models.Signal {
	hist := exchange.GetCandleHistory(symbol)
	highs := hist.GetHighs()
	lows := hist.GetLows()
	closes := hist.GetCloses()
	volumes := hist.GetVolumes()

	n := len(closes)
	if n < 2*s.AtrPeriod || n < s.VolLen {
		return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
	}

	// --- ATR calculation ---
	atr := talib.Atr(highs, lows, closes, s.AtrPeriod)

	// --- Supertrend calculation ---
	W := 2 * s.AtrPeriod
	if W > n {
		W = n
	}
	start := n - W

	up := make([]float64, W)
	dn := make([]float64, W)
	trend := make([]int, W)

	for i := 0; i < W; i++ {
		idx := start + i

		src := (highs[idx] + lows[idx]) / 2.0
		up[i] = src - s.Factor*atr[idx]
		dn[i] = src + s.Factor*atr[idx]

		if i == 0 {
			trend[i] = 1 // initial trend bullish
			continue
		}

		// sticky band adjustments
		if closes[idx-1] > up[i-1] {
			up[i] = math.Max(up[i], up[i-1])
		}
		if closes[idx-1] < dn[i-1] {
			dn[i] = math.Min(dn[i], dn[i-1])
		}

		// trend flip logic
		if trend[i-1] == -1 && closes[idx] > dn[i-1] {
			trend[i] = 1
		} else if trend[i-1] == 1 && closes[idx] < up[i-1] {
			trend[i] = -1
		} else {
			trend[i] = trend[i-1]
		}
	}

	// --- Volume filter ---
	volMA := talib.Sma(volumes, s.VolLen)
	volFilter := volumes[n-1] > volMA[len(volMA)-1]

	// --- Current state ---
	closeCurr := closes[n-1]
	state := s.PositionHolder.State[symbol]
	inPosition := state.InPosition

	atrVal := atr[len(atr)-1]
	trailingStop := closeCurr - s.TsAtrMult*atrVal
	takeProfit := closeCurr + s.TpAtrMult*atrVal

	// --- Entry / Exit ---
	last := W - 1
	prev := W - 2

	buyFlip := trend[last] == 1 && trend[prev] == -1
	sellFlip := trend[last] == -1 && trend[prev] == 1

	// Buy if supertrend flips bullish and not already in a position
	if buyFlip && !inPosition && (!s.UseVolFilt || volFilter) {
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

	// Exit if in a long and either trend flips bearish or stop/TP is hit
	if inPosition {
		isReachedTakeProfit := closeCurr >= state.TakeProfit
		isReachedTrailingStop := closeCurr <= state.TrailingStop

		if isReachedTakeProfit || isReachedTrailingStop || (sellFlip && (!s.UseVolFilt || volFilter)) {
			return models.Signal{
				Symbol:  symbol,
				Type:    enum.SignalSell,
				Percent: 100,
				Time:    time.Now(),
				Price:   closeCurr,
			}
		}
	}

	// Hold otherwise
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

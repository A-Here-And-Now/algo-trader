package strategies

import (
	"fmt"
	"math"
	"strconv"
	"time"

	helper "github.com/A-Here-And-Now/algo-trader/orchestration_api/entities/signaler/strategy_helper"
	enum "github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	models "github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
	talib "github.com/markcheno/go-talib"
)

type SupertrendStrategy struct{ *helper.PositionHolder }

func (s *SupertrendStrategy) CalculateSignal(symbol string, priceStore helper.IPriceActionStore) models.Signal {
	// === Default inputs (match Pine defaults) ===
	atrPeriod := 11
	factor := 2.0
	orbHour := 9
	orbMinute := 30
	gmt := "America/New_York"
	orbLength := 5
	volumeFilterEnabled := true
	volumeLength := 50
	riskRewardRatio := 3.0

	// --- read history ---
	hist := priceStore.GetCandleHistory(symbol)
	highs := hist.GetHighs()
	lows := hist.GetLows()
	closes := hist.GetCloses()
	volumes := hist.GetVolumes()
	starts := hist.GetStarts() // []time.Time - adapt if your API uses a different name/type

	n := len(closes)
	// need at least some data
	if n < 2 {
		return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
	}

	// --- ATR and volume MA via talib ---
	atrArr := talib.Atr(highs, lows, closes, atrPeriod)
	volSMA := talib.Sma(volumes, volumeLength)

	// --- SuperTrend implementation (standard algorithm) ---
	// basic bands
	basicUpper := make([]float64, n)
	basicLower := make([]float64, n)
	finalUpper := make([]float64, n)
	finalLower := make([]float64, n)
	superTrend := make([]float64, n)
	dir := make([]int, n) // -1 = up (bullish), +1 = down (bearish) — we will follow the PineScript usage where they check direction < 0 for entry

	for i := 0; i < n; i++ {
		atrv := math.NaN()
		if i < len(atrArr) {
			atrv = atrArr[i]
		}
		mid := (highs[i] + lows[i]) / 2.0
		if !math.IsNaN(atrv) {
			basicUpper[i] = mid + factor*atrv
			basicLower[i] = mid - factor*atrv
		} else {
			basicUpper[i] = math.NaN()
			basicLower[i] = math.NaN()
		}

		// initialize first values
		if i == 0 {
			finalUpper[i] = basicUpper[i]
			finalLower[i] = basicLower[i]
			dir[i] = 1 // default to down/bear (so direction < 0 is bullish later)
			superTrend[i] = finalUpper[i]
			continue
		}

		// final upper/lower follow the standard SuperTrend logic
		if !math.IsNaN(basicUpper[i]) && !math.IsNaN(finalUpper[i-1]) {
			if basicUpper[i] < finalUpper[i-1] || closes[i-1] > finalUpper[i-1] {
				finalUpper[i] = basicUpper[i]
			} else {
				finalUpper[i] = finalUpper[i-1]
			}
		} else {
			finalUpper[i] = basicUpper[i]
		}

		if !math.IsNaN(basicLower[i]) && !math.IsNaN(finalLower[i-1]) {
			if basicLower[i] > finalLower[i-1] || closes[i-1] < finalLower[i-1] {
				finalLower[i] = basicLower[i]
			} else {
				finalLower[i] = finalLower[i-1]
			}
		} else {
			finalLower[i] = basicLower[i]
		}

		// direction switching logic:
		// if previous direction was down (1) and close <= finalUpper => remain down (1)
		// if previous direction was down (1) and close > finalUpper => switch to up (-1)
		// if previous direction was up (-1) and close >= finalLower => remain up (-1)
		// if previous direction was up (-1) and close < finalLower => switch to down (1)
		prevDir := dir[i-1]
		if prevDir == 1 {
			if closes[i] > finalUpper[i] {
				dir[i] = -1 // switch to up (we choose -1 to match Pine direction < 0 used in original)
			} else {
				dir[i] = 1
			}
		} else {
			if closes[i] < finalLower[i] {
				dir[i] = 1 // switch to down
			} else {
				dir[i] = -1
			}
		}

		// supertrend line is finalUpper when direction is down (1), finalLower when direction is up (-1)
		if dir[i] == 1 {
			superTrend[i] = finalUpper[i]
		} else {
			superTrend[i] = finalLower[i]
		}
	}

	// last values
	last := n - 1
	if last < 1 {
		return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
	}
	superVal := superTrend[last]
	superDir := dir[last] // direction: -1 => up, 1 => down; original pine used direction<0 in long condition

	// --- ORB calculation: find most recent session start index and aggregate the next orbLength candles' high/low ---
	loc, _ := time.LoadLocation(gmt)
	var sessionStartIdx int = -1
	for i := n - 1; i >= 0; i-- {
		if i >= len(starts) {
			continue
		}
		unix, err := strconv.ParseInt(starts[i], 10, 64)
		if err != nil {
			panic(fmt.Sprintf("agghh: %s failed to parse start time in supertrend: %s - %s", symbol, starts[i], err))
		}
		t := time.Unix(unix, 0).In(loc)
		if t.Hour() == orbHour && t.Minute() == orbMinute {
			sessionStartIdx = i
			break
		}
	}
	var sHigh, sLow float64
	sHigh = math.NaN()
	sLow = math.NaN()
	if sessionStartIdx >= 0 && sessionStartIdx+orbLength <= n {
		// collect highs/lows for orbLength candles starting at sessionStartIdx
		hmax := highs[sessionStartIdx]
		lmin := lows[sessionStartIdx]
		for k := sessionStartIdx; k < sessionStartIdx+orbLength; k++ {
			if highs[k] > hmax {
				hmax = highs[k]
			}
			if lows[k] < lmin {
				lmin = lows[k]
			}
		}
		sHigh = hmax
		sLow = lmin
	}

	// --- Volume filter check ---
	volumeOK := true
	if volumeFilterEnabled {
		// ensure we have SMA value
		if last < len(volSMA) && !math.IsNaN(volSMA[last]) {
			volumeOK = volumes[last] > volSMA[last]
		} else {
			volumeOK = false
		}
	}

	// --- Entry / exit logic ---
	closeCurr := closes[last]
	closePrev := closes[last-1]

	// long_condition = close > s_high and close[1] <= s_high and close > open and direction < 0
	// Note: we don't have open here in this snippet; use hist.GetOpens() if available
	opens := hist.GetOpens()
	longCond := false
	if !math.IsNaN(sHigh) {
		if closeCurr > sHigh && closePrev <= sHigh && closeCurr > opens[last] && superDir < 0 {
			longCond = true
		}
	}
	if volumeFilterEnabled {
		longCond = longCond && volumeOK
	}

	// short_condition = close < s_low and close[1] >= s_low and close < open and direction > 0
	// We will NOT enter new shorts; we only use the short condition to close longs per your rules.
	shortCond := false
	if !math.IsNaN(sLow) {
		if closeCurr < sLow && closePrev >= sLow && closeCurr < opens[last] && superDir > 0 {
			shortCond = true
		}
	}

	// Exit on opposite breakout:
	exitLongCond := false
	if !math.IsNaN(sLow) {
		exitLongCond = closeCurr < sLow
	}

	// Stop loss and take profit calculation (use s_low / s_high as base, but user requested different multipliers)
	// We will apply multipliers to an ATR-based buffer similar to your other scripts
	atrVal := math.NaN()
	if len(atrArr) > 0 {
		atrVal = atrArr[last]
	}

	// defaults for multipliers (you said don't check then set — so hard-coded here)
	tsAtrMult := 1.0 // trailing stop multiplier (applied to ATR for buffer, but Pine used s_low directly)
	tpAtrMult := riskRewardRatio // Reuse riskRewardRatio as TP multiplier relative to stop distance

	// determine trailingStop and takeProfit values for a long entry
	var trailingStop, takeProfit float64
	if !math.IsNaN(sLow) {
		// choose a stop anchored at s_low with optional ATR buffer
		if !math.IsNaN(atrVal) {
			// anchor at s_low but push it a little lower by ATR*tsAtrMult for breathing room
			trailingStop = sLow - tsAtrMult*atrVal
		} else {
			trailingStop = sLow
		}
		// takeProfit as riskRewardRatio * (entry - stop)
		stopDistance := closeCurr - trailingStop
		takeProfit = closeCurr + tpAtrMult*stopDistance
	}

	// --- Position state checks ---
	state := s.PositionHolder.State[symbol]
	inPosition := state.InPosition

	// Entry: only create long entries (no shorts). Use full 100% allocation per your latest pattern.
	if longCond && !inPosition {
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

	// If in position, check for TP or TrailingStop or opposite breakout to close
	if inPosition {
		isReachedTP := !math.IsNaN(state.TakeProfit) && closeCurr >= state.TakeProfit
		isReachedTS := !math.IsNaN(state.TrailingStop) && closeCurr <= state.TrailingStop
		if isReachedTP || isReachedTS || exitLongCond {
			return models.Signal{
				Symbol:  symbol,
				Type:    enum.SignalSell,
				Percent: 100,
				Time:    time.Now(),
				Price:   closeCurr,
			}
		}
	}

	// If we are not in position and a short condition happens, ignore (no new shorts).
	// If we are in position and shortCond triggers, close (handled above as exitLongCond / exit logic).

	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

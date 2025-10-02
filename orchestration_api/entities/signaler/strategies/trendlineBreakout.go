package strategies

import (
	"math"
	"time"

	helper "github.com/A-Here-And-Now/algo-trader/orchestration_api/entities/signaler/strategy_helper"
	enum "github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	models "github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
	talib "github.com/markcheno/go-talib"
)

type TrendlineBreakoutStrategy struct {
	*helper.PositionHolder
	PivLR    		int
	UseEmaFilter    bool
	EmaLen    		int
	AtrLen   		int
	TsAtrMult  		float64
	TpAtrMult  		float64
}

func (s *TrendlineBreakoutStrategy) CalculateSignal(symbol string, priceStore helper.IPriceActionStore) models.Signal {
	hist := priceStore.GetCandleHistory(symbol)
	highs := hist.GetHighs()
	lows := hist.GetLows()
	closes := hist.GetCloses()

	n := len(closes)
	// need enough bars to confirm pivot (pivLR on both sides) + safety
	if n < s.PivLR+2 {
		return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
	}

	ema := talib.Ema(closes, s.EmaLen)
	atr := talib.Atr(highs, lows, closes, s.AtrLen)

	// --- pivot detection (confirmed pivot when center bar is min/max over window [i-pivLR .. i+pivLR]) ---
	// store confirmed pivot indices & prices like the Pine arrays pl_x / pl_y, ph_x / ph_y
	var plX []int
	var plY []float64
	var phX []int
	var phY []float64

	// We can only check pivots for center positions between pivLR and n-pivLR-1 inclusive
	// center i corresponds to "bar_index" position i
	for center := s.PivLR; center <= n-1-s.PivLR; center++ {
		left := center - s.PivLR
		right := center + s.PivLR
		// pivot low (lowest low in window)
		windowMin := helper.MinSlice(lows, left, right)
		if lows[center] == windowMin {
			// push pivot low at center
			plX = append(plX, center)
			plY = append(plY, lows[center])
			// trim to last 20 like Pine
			if len(plX) > 20 {
				plX = plX[1:]
				plY = plY[1:]
			}
		}
		// pivot high (highest high in window)
		windowMax := helper.MaxSlice(highs, left, right)
		if highs[center] == windowMax {
			phX = append(phX, center)
			phY = append(phY, highs[center])
			if len(phX) > 20 {
				phX = phX[1:]
				phY = phY[1:]
			}
		}
	}

	// --- Trendline construction (use last two confirmed pivots) ---
	var upLineExists bool
	var upX1, upX2 int
	var upY1, upY2 float64

	if len(plX) >= 2 {
		ux1 := plX[len(plX)-2]
		ux2 := plX[len(plX)-1]
		uy1 := plY[len(plY)-2]
		uy2 := plY[len(plY)-1]
		// require higher lows and increasing index like Pine
		if uy2 > uy1 && ux2 > ux1 {
			upLineExists = true
			upX1, upX2 = ux1, ux2
			upY1, upY2 = uy1, uy2
		}
	}

	var downLineExists bool
	var downX1, downX2 int
	var downY1, downY2 float64

	if len(phX) >= 2 {
		dx1 := phX[len(phX)-2]
		dx2 := phX[len(phX)-1]
		dy1 := phY[len(phY)-2]
		dy2 := phY[len(phY)-1]
		// require lower highs and increasing index like Pine
		if dy2 < dy1 && dx2 > dx1 {
			downLineExists = true
			downX1, downX2 = dx1, dx2
			downY1, downY2 = dy1, dy2
		}
	}

	// current bar index (using array index as bar index)
	currIdx := float64(n - 1)
	prevIdx := float64(n - 2)

	var upPriceCurr, upPricePrev float64
	var downPriceCurr, downPricePrev float64
	if upLineExists {
		upPriceCurr = helper.LinePriceAt(float64(upX1), upY1, float64(upX2), upY2, currIdx)
		upPricePrev = helper.LinePriceAt(float64(upX1), upY1, float64(upX2), upY2, prevIdx)
	} else {
		upPriceCurr = math.NaN()
		upPricePrev = math.NaN()
	}
	if downLineExists {
		downPriceCurr = helper.LinePriceAt(float64(downX1), downY1, float64(downX2), downY2, currIdx)
		downPricePrev = helper.LinePriceAt(float64(downX1), downY1, float64(downX2), downY2, prevIdx)
	} else {
		downPriceCurr = math.NaN()
		downPricePrev = math.NaN()
	}

	// --- crossover / crossunder logic (use previous bar values like Pine's ta.crossover/ta.crossunder) ---
	closeCurr := closes[n-1]
	closePrev := closes[n-2]

	crossAboveDown := false
	crossBelowUp := false
	if !math.IsNaN(downPriceCurr) && !math.IsNaN(downPricePrev) {
		crossAboveDown = closePrev <= downPricePrev && closeCurr > downPriceCurr
	}
	if !math.IsNaN(upPriceCurr) && !math.IsNaN(upPricePrev) {
		crossBelowUp = closePrev >= upPricePrev && closeCurr < upPriceCurr
	}

	// --- Gate with MA filter (use EMA series from talib; talib.Ema returns array where first len < maLen may be zero)
	maVal := math.NaN()
	if len(ema) > 0 {
		maVal = ema[len(ema)-1]
	}

	longBreak := !math.IsNaN(downPriceCurr) && crossAboveDown && (!s.UseEmaFilter || closeCurr > maVal)
	shortBreak := !math.IsNaN(upPriceCurr) && crossBelowUp && (!s.UseEmaFilter || closeCurr < maVal)

	// --- Position state checks (use PositionHolder.State) ---
	state := s.PositionHolder.State[symbol]
	inPosition := state.InPosition

	atrVal := atr[len(atr)-1]

	trailingStop := closeCurr - s.TsAtrMult*atrVal
	takeProfit := closeCurr + s.TpAtrMult*atrVal

	// --- Signals & returns ---
	// If not in position and longBreak -> enter long (Percent = 10 per Pine default)
	if longBreak && !inPosition {
		return models.Signal{
			Symbol:       symbol,
			Type:         enum.SignalBuy,
			Percent:      100, 
			Time:         time.Now(),
			TrailingStop: trailingStop,
			TakeProfit:   takeProfit,
			Price:        closeCurr,
		}
	} else if inPosition && !shortBreak {	
		isReachedTakeProfit := closeCurr >= state.TakeProfit
		isReachedTrailingStop := closeCurr <= state.TrailingStop
		if isReachedTakeProfit || isReachedTrailingStop {
			return models.Signal{
				Symbol:  symbol,
				Type:    enum.SignalSell,
				Percent: 100,
				Time:    time.Now(),
				Price:   closeCurr,
			}
		}
	} else if shortBreak {
		return models.Signal{
			Symbol:       symbol,
			Type:         enum.SignalSell, // treat as opening a short; if your infra doesn't short, you'll treat it as close
			Percent:      100,
			Time:         time.Now(),
			Price:        closeCurr,
		}
	}

	// No actionable event
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

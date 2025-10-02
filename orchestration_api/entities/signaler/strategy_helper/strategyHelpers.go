package strategy_helper

import (
	"math"

	talib "github.com/markcheno/go-talib"
)

// ----- SMA / EMA / SMMA -------------------------------------------------------
func Sma(src []float64, length int) []float64 { return talib.Sma(src, length) }
func Ema(src []float64, length int) []float64 { return talib.Ema(src, length) }

// SMMA – recursive smoothing, first value = SMA
func Smma(src []float64, length int) []float64 {
	out := make([]float64, len(src))
	if len(src) == 0 {
		return out
	}
	// first value is a simple SMA of the first <length> bars
	if len(src) < length {
		out[0] = talib.Sma(src, len(src))[len(src)-1]
	} else {
		out[0] = talib.Sma(src[:length], length)[length-1]
	}
	for i := 1; i < len(src); i++ {
		out[i] = (out[i-1]*float64(length-1) + src[i]) / float64(length)
	}
	return out
}

// Pick the MA type requested by the script (here it will always be SMA)
func CalcMA(src []float64, length int, typ string) []float64 {
	switch typ {
	case "EMA":
		return Ema(src, length)
	case "SMMA":
		return Smma(src, length)
	default: // SMA
		return Sma(src, length)
	}
}

// ----- cross‑over helpers ----------------------------------------------------
func CrossOver(a, b []float64) bool {
	if len(a) < 2 || len(b) < 2 {
		return false
	}
	return a[len(a)-2] < b[len(b)-2] && a[len(a)-1] > b[len(b)-1]
}

func CrossUnder(a, b []float64) bool {
	if len(a) < 2 || len(b) < 2 {
		return false
	}
	return a[len(a)-2] > b[len(b)-2] && a[len(a)-1] < b[len(b)-1]
}

func BodySize(o, c float64) float64 { return math.Abs(c - o) }
func CandleRange(h, l float64) float64 { return h - l }
func UpperShadow(o, c, h float64) float64 {
    if math.Max(o, c) < h {
        return h - math.Max(o, c)
    }
    return 0
}

func LowerShadow(o, c, l float64) float64 {
    if math.Min(o, c) > l {
        return math.Min(o, c) - l
    }
    return 0
}

func IsBullish(o, c float64) bool { return c > o }
func IsBearish(o, c float64) bool { return c < o }

// ----- ATR‑aware body‑size helpers -------------------------------------------
func IsSmallBody(o, c, h, l, atr float64, mult float64, perc float64) bool {
    // “enableATRFilter” is always true in the original script (default = true)
    return BodySize(o, c) < atr*mult ||
        BodySize(o, c) < CandleRange(h, l)*perc
}

func IsLongBody(o, c, h, l, atr float64, mult float64, perc float64) bool {
    return BodySize(o, c) > atr*mult ||
        BodySize(o, c) > CandleRange(h, l)*perc
}

// ----- pattern‑specific helpers ----------------------------------------------
func IsDoji(o, c, h, l, atr float64, mult float64, perc float64) bool {
    return IsSmallBody(o, c, h, l, atr, mult, perc) &&
        UpperShadow(o, c, h) > 0 && LowerShadow(o, c, l) > 0
}

func IsHaramiStrict(o, c, h, l, o1, c1, h1, l1 float64) bool {
    return h < h1 && l > l1 // current candle fully inside previous candle
}

func IsEngulfing(o, c, o1, c1 float64) bool {
    return BodySize(o, c) > BodySize(o1, c1) &&
        IsBullish(o, c) != IsBullish(o1, c1)
}

func IsMarubozu(o, c, h, l float64, perc float64) bool {
	return BodySize(o, c) > CandleRange(h, l)*(1-perc)
}

// IsGapUp returns true when the current open opens **above** the high of the
// previous candle (i.e. a bullish gap).
func IsGapUp(curOpen, prevHigh float64) bool {
    // A gap exists only if the price actually moved; we also protect against
    // NaN/Inf values that sometimes appear in a freshly‑initialized slice.
    if math.IsNaN(curOpen) || math.IsNaN(prevHigh) {
        return false
    }
    return curOpen > prevHigh
}

// IsGapDown returns true when the current open opens **below** the low of the
// previous candle (i.e. a bearish gap).
func IsGapDown(curOpen, prevLow float64) bool {
    if math.IsNaN(curOpen) || math.IsNaN(prevLow) {
        return false
    }
    return curOpen < prevLow
}

// ---------------------------------------------------------------------
//  Shadow‑percentage helper – used to test whether a shadow is “long”.
// ---------------------------------------------------------------------

// IsLongShadowPercent checks whether a shadow (upper or lower) is at least
// `perc` (expressed as a fraction, e.g. 0.3 for 30 %) of the total candle range.
// `shadow` = the size of the shadow you want to test.
// `candleRange` = high – low of the candle.
func IsLongShadowPercent(shadow, candleRange, perc float64) bool {
    if candleRange == 0 {
        // avoid division‑by‑zero; a zero‑range candle can’t have a “long” shadow.
        return false
    }
    return shadow > candleRange*perc
}

func GetHLPivot(highs []float64, lows []float64, swingPivotLength int) (float64, float64) {
	ph := float64(0)
	pl := float64(0)
	for i := len(highs) - 1; i >= 0 && i > swingPivotLength; i-- {
		// look‑back `swingPivotLength` bars on each side
		highPeak := true
		lowTrough := true
		for j := 1; j <= swingPivotLength; j++ {
			if highs[i] <= highs[i-j] || highs[i] <= highs[i+j] {
				highPeak = false
			}
			if lows[i] >= lows[i-j] || lows[i] >= lows[i+j] {
				lowTrough = false
			}
		}
		if highPeak && math.IsNaN(ph) {
			ph = highs[i]
		}
		if lowTrough && math.IsNaN(pl) {
			pl = lows[i]
		}
		if !math.IsNaN(ph) && !math.IsNaN(pl) {
			break
		}
	}
	return ph, pl
}

func LinePriceAt(x1 float64, y1 float64, x2 float64, y2 float64, idx float64) float64 {
	if x2 == x1 {
		return y2
	}
	slope := (y2 - y1) / (x2 - x1)
	return y2 + slope*(idx-x2)
}

func MinSlice(slice []float64, from, to int) float64 {
	m := slice[from]
	for k := from + 1; k <= to; k++ {
		if slice[k] < m {
			m = slice[k]
		}
	}
	return m
}

func MaxSlice(slice []float64, from, to int) float64 {
	m := slice[from]
	for k := from + 1; k <= to; k++ {
		if slice[k] > m {
			m = slice[k]
		}
	}
	return m
}

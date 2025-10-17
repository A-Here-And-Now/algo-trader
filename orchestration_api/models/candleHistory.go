package models

import (
	"math"
	"time"
)

type CandleHistory struct {
	Candles []Candle
}

type RenkoCandleHistory struct {
	RenkoCandles []RenkoCandle
	LastCandlePrice float64
	BrickSize float64
}

type HeikenAshiCandleHistory struct {
	HeikenAshiCandles []HeikenAshiCandle
}

func NewCandleHistory(candles []Candle) *CandleHistory {
	return &CandleHistory{
		Candles: candles,
	}
}

func (c *CandleHistory) GetHeikenAshiCandleHistory() HeikenAshiCandleHistory {
	if len(c.Candles) == 0 {
		return HeikenAshiCandleHistory{
			HeikenAshiCandles: nil,
		}
	}

	haCandles := make([]HeikenAshiCandle, len(c.Candles))

	// First candle special case
	first := c.Candles[0]
	haClose := (first.Open + first.High + first.Low + first.Close) / 4
	haOpen := (first.Open + first.Close) / 2
	haHigh := math.Max(first.High, math.Max(haOpen, haClose))
	haLow := math.Min(first.Low, math.Min(haOpen, haClose))

	haCandles[0] = HeikenAshiCandle{
		Start: first.Start,
		Open:  haOpen,
		Close: haClose,
		High:  haHigh,
		Low:   haLow,
		Volume: first.Volume,
	}

	// Rest of candles
	for i := 1; i < len(c.Candles); i++ {
		c := c.Candles[i]
		haClose = (c.Open + c.High + c.Low + c.Close) / 4
		prev := haCandles[i-1]
		haOpen = (prev.Open + prev.Close) / 2
		haHigh = math.Max(c.High, math.Max(haOpen, haClose))
		haLow = math.Min(c.Low, math.Min(haOpen, haClose))

		haCandles[i] = HeikenAshiCandle{
			Start: c.Start,
			Open:  haOpen,
			Close: haClose,
			High:  haHigh,
			Low:   haLow,
			Volume: c.Volume,
		}
	}

	return HeikenAshiCandleHistory{
		HeikenAshiCandles: haCandles,
	}
}

func (c *CandleHistory) GetLows() []float64 {
	lows := make([]float64, len(c.Candles))
	for i, candle := range c.Candles {
		lows[i] = candle.Low
	}
	return lows
}

func (c *CandleHistory) GetHighs() []float64 {
	highs := make([]float64, len(c.Candles))
	for i, candle := range c.Candles {
		highs[i] = candle.High
	}
	return highs
}

func (c *CandleHistory) GetCloses() []float64 {
	closes := make([]float64, len(c.Candles))
	for i, candle := range c.Candles {
		closes[i] = candle.Close
	}
	return closes
}

func (c *CandleHistory) GetVolumes() []float64 {
	volumes := make([]float64, len(c.Candles))
	for i, candle := range c.Candles {
		volumes[i] = candle.Volume
	}
	return volumes
}

func (c *CandleHistory) GetStarts() []time.Time {
	starts := make([]time.Time, len(c.Candles))
	for i, candle := range c.Candles {
		starts[i] = candle.Start
	}
	return starts
}

func (c *CandleHistory) GetOpens() []float64 {
	opens := make([]float64, len(c.Candles))
	for i, candle := range c.Candles {
		opens[i] = candle.Open
	}
	return opens
}

func (c *RenkoCandleHistory) GetRenkoCloses() []float64 {
	closes := make([]float64, len(c.RenkoCandles))
	for i, candle := range c.RenkoCandles {
		closes[i] = candle.Close
	}
	return closes
}

func (c *RenkoCandleHistory) GetRenkoOpens() []float64 {
	opens := make([]float64, len(c.RenkoCandles))
	for i, candle := range c.RenkoCandles {
		opens[i] = candle.Open
	}
	return opens
}

func (c *HeikenAshiCandleHistory) GetHeikenAshiLows() []float64 {
	lows := make([]float64, len(c.HeikenAshiCandles))
	for i, candle := range c.HeikenAshiCandles {
		lows[i] = candle.Low
	}
	return lows
}

func (c *HeikenAshiCandleHistory) GetHeikenAshiCloses() []float64 {
	closes := make([]float64, len(c.HeikenAshiCandles))
	for i, candle := range c.HeikenAshiCandles {
		closes[i] = candle.Close
	}
	return closes
}

func (c *HeikenAshiCandleHistory) GetHeikenAshiHighs() []float64 {
	highs := make([]float64, len(c.HeikenAshiCandles))
	for i, candle := range c.HeikenAshiCandles {
		highs[i] = candle.High
	}
	return highs
}

func (c *HeikenAshiCandleHistory) GetHeikenAshiVolumes() []float64 {
	volumes := make([]float64, len(c.HeikenAshiCandles))
	for i, candle := range c.HeikenAshiCandles {
		volumes[i] = candle.Volume
	}
	return volumes
}

func (c *HeikenAshiCandleHistory) GetHeikenAshiStarts() []time.Time {
	starts := make([]time.Time, len(c.HeikenAshiCandles))
	for i, candle := range c.HeikenAshiCandles {
		starts[i] = candle.Start
	}
	return starts
}

func (c *HeikenAshiCandleHistory) GetHeikenAshiOpens() []float64 {
	opens := make([]float64, len(c.HeikenAshiCandles))
	for i, candle := range c.HeikenAshiCandles {
		opens[i] = candle.Open
	}
	return opens
}
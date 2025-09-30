package signaler

import "github.com/A-Here-And-Now/algo-trader/orchestration_api/models"

type CandleHistory struct {
	Candles []models.Candle
}

func NewCandleHistory(candles []models.Candle) *CandleHistory {
	return &CandleHistory{
		Candles: candles,
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

func (c *CandleHistory) GetStarts() []string {
	starts := make([]string, len(c.Candles))
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
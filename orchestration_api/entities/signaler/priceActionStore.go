// signaler/market_store.go
package signaler

import (
	"math"
	"sync"
	// "sync"
	"time"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
)

// ------------------------------------------------------------
// Public, read‑only view that strategies receive.
// ------------------------------------------------------------
type PriceActionStore interface {
	// Return a **copy** of the slice so a strategy cannot modify our
	// internal buffer.  The copy is cheap because the underlying array is
	// shared – only the slice header is duplicated.
	GetPriceHistory(symbol string) []models.Ticker
	GetCandleHistory(symbol string) CandleHistory
	GetCandleHistory26Days(symbol string) CandleHistory
	GetFullMergedCandleHistory(symbol string) CandleHistory
}

// ------------------------------------------------------------
// The concrete store that the engine updates.
// ------------------------------------------------------------
type priceActionStore struct {
	mu            sync.RWMutex
	tokens        []string
	priceHistory  map[string][]models.Ticker
	candleHistory map[string]*CandleHistory
	candleHistory26Days map[string]*CandleHistory
	fiveMinuteCandleCounter map[string]int
	periodLength time.Duration
	numShortEmaPeriods int
	numLongEmaPeriods int
}

// NewStore creates the empty structures.
func NewStore(strategy enum.Strategy) *priceActionStore {
	store := priceActionStore{
		tokens:        []string{},
		priceHistory:  make(map[string][]models.Ticker),
		candleHistory: make(map[string]*CandleHistory),
		candleHistory26Days: make(map[string]*CandleHistory),
		fiveMinuteCandleCounter: make(map[string]int),
		periodLength: 2 * time.Hour,
		numShortEmaPeriods: 12,
		numLongEmaPeriods: 26,
	}

	return &store
}

func (s *priceActionStore) AddToken(symbol string, priceHistory []models.Ticker, candleHistory []models.Candle, candleHistory26Days []models.Candle) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens = append(s.tokens, symbol)
	s.priceHistory[symbol] = priceHistory
	s.candleHistory[symbol] = NewCandleHistory(candleHistory)
	s.candleHistory26Days[symbol] = NewCandleHistory(candleHistory26Days)
	s.fiveMinuteCandleCounter[symbol] = 0
}

func (s *priceActionStore) RemoveToken(symbol string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.priceHistory, symbol)
	delete(s.candleHistory, symbol)
	delete(s.candleHistory26Days, symbol)
	delete(s.fiveMinuteCandleCounter, symbol)
	idx := 0
	for i, t := range s.tokens {
		if t == symbol {
			idx = i
			break
		}
	}
	s.tokens = append(s.tokens[:idx], s.tokens[idx+1:]...)
}

func (s *priceActionStore) IngestPrice(symbol string, price models.Ticker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.priceHistory[symbol] = append(s.priceHistory[symbol], price)
	// keep only last N ticks for memory safety
	buf := s.priceHistory[symbol]
	const maxTicks = 1200 // ~10 minutes if ~2/s
	if len(buf) > maxTicks {
		buf = buf[len(buf)-maxTicks:]
	}
	s.priceHistory[symbol] = buf
}

func (s *priceActionStore) IngestCandle(symbol string, candle models.Candle) {
	s.mu.Lock()
	defer s.mu.Unlock()
	length := len(s.candleHistory[symbol].Candles)
	if (s.candleHistory[symbol].Candles[length-1].Start == candle.Start) {
		s.candleHistory[symbol].Candles[length-1] = candle
	} else {
		s.candleHistory[symbol].Candles = append(s.candleHistory[symbol].Candles, candle)
		if len(s.candleHistory[symbol].Candles) > 24 {
			s.fiveMinuteCandleCounter[symbol]++
			s.candleHistory[symbol].Candles = s.candleHistory[symbol].Candles[1:]
			if s.fiveMinuteCandleCounter[symbol] >= 24 {
				s.fiveMinuteCandleCounter[symbol] = 0
				s.Shift26DayCandleHistory(symbol, s.candleHistory[symbol].Candles)
			}
		}
	}
}


func (s *priceActionStore) Shift26DayCandleHistory(symbol string, fiveMinuteCandles []models.Candle) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var newHistory models.Candle
	newHistory.Start = fiveMinuteCandles[0].Start
	newHistory.Open = fiveMinuteCandles[0].Open
	newHistory.Close = fiveMinuteCandles[len(fiveMinuteCandles)-1].Close
	newHistory.ProductID = fiveMinuteCandles[0].ProductID
	
	for _, candle := range fiveMinuteCandles {
		if candle.Low < newHistory.Low {
			newHistory.Low = candle.Low
		}
		if candle.High > newHistory.High {
			newHistory.High = candle.High
		}
		newHistory.Volume += candle.Volume
	}
	s.candleHistory26Days[symbol].Candles = append(s.candleHistory26Days[symbol].Candles, newHistory)
	s.candleHistory26Days[symbol].Candles = s.candleHistory26Days[symbol].Candles[1:]
}

func (s *priceActionStore) GetPriceHistory(symbol string) []models.Ticker {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.priceHistory[symbol]
}

func (s *priceActionStore) GetCandleHistory(symbol string) CandleHistory {
	s.mu.RLock()
	defer s.mu.RUnlock()
    orig := s.candleHistory[symbol]

    merged := make([]models.Candle, 0, len(orig.Candles))
    merged = append(merged, orig.Candles...)

    return CandleHistory{Candles: merged}
}

func (s *priceActionStore) GetCandleHistory26Days(symbol string) CandleHistory {
	s.mu.RLock()
	defer s.mu.RUnlock()
    orig := s.candleHistory26Days[symbol]

    merged := make([]models.Candle, 0, len(orig.Candles))
    merged = append(merged, orig.Candles...)

    return CandleHistory{Candles: merged}
}

func (s *priceActionStore) GetFullMergedCandleHistory(symbol string) CandleHistory {
	s.mu.RLock()
	defer s.mu.RUnlock()
    orig := s.candleHistory26Days[symbol]

    merged := make([]models.Candle, 0, len(orig.Candles))
    merged = append(merged, orig.Candles...) // copy the existing data

	if len(s.candleHistory[symbol].Candles) > 0 {
		merged = append(merged, s.getTwoHourCandleFromShortHistory(symbol))
	}

    return CandleHistory{Candles: merged}
}

func (s *priceActionStore) getTwoHourCandleFromShortHistory(symbol string) models.Candle {
	new2HourCandle := models.Candle{
		Start: s.candleHistory[symbol].Candles[0].Start,
		Open: s.candleHistory[symbol].Candles[0].Open,
		Close: s.candleHistory[symbol].Candles[0].Close,
	}
	for _, candle := range s.candleHistory[symbol].Candles {
		new2HourCandle.High = math.Max(new2HourCandle.High, candle.High)
		new2HourCandle.Low = math.Min(new2HourCandle.Low, candle.Low)
		new2HourCandle.Volume += candle.Volume
	}
	return new2HourCandle
}

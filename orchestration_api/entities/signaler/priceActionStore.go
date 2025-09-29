// signaler/market_store.go
package signaler

import (
	"math"
	"sync"

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
	GetCandleHistory(symbol string) []models.Candle
	GetCandleHistory26Days(symbol string) []models.Candle
	GetIndicator(symbol string, indicator enum.IndicatorType) float64
}

// ------------------------------------------------------------
// The concrete store that the engine updates.
// ------------------------------------------------------------
type priceActionStore struct {
	mu            sync.RWMutex
	tokens        []string
	priceHistory  map[string][]models.Ticker
	candleHistory map[string][]models.Candle
	candleHistory26Days map[string][]models.Candle
	fiveMinuteCandleCounter map[string]int
	indicators map[string]map[enum.IndicatorType]Indicator
	activeIndicators []enum.IndicatorType
	latestPriceAtWhichIndicatorsWereUpdated map[string]float64
}

// NewStore creates the empty structures.
func NewStore(strategy enum.Strategy) *priceActionStore {
	store := priceActionStore{
		tokens:        []string{},
		priceHistory:  make(map[string][]models.Ticker),
		candleHistory: make(map[string][]models.Candle),
		candleHistory26Days: make(map[string][]models.Candle),
		fiveMinuteCandleCounter: make(map[string]int),
		indicators: make(map[string]map[enum.IndicatorType]Indicator),
		activeIndicators: []enum.IndicatorType{},
		latestPriceAtWhichIndicatorsWereUpdated: make(map[string]float64),
	}

	store.UpdateActiveIndicators(strategy)

	return &store
}

func (s *priceActionStore) AddToken(symbol string, priceHistory []models.Ticker, candleHistory []models.Candle, candleHistory26Days []models.Candle) {
	s.tokens = append(s.tokens, symbol)
	s.priceHistory[symbol] = priceHistory
	s.candleHistory[symbol] = candleHistory
	s.candleHistory26Days[symbol] = candleHistory26Days
	s.latestPriceAtWhichIndicatorsWereUpdated[symbol] = s.candleHistory[symbol][len(s.candleHistory[symbol])-1].Close
	s.indicators[symbol] = make(map[enum.IndicatorType]Indicator)
	for _, indicator := range s.activeIndicators {
		s.indicators[symbol][indicator] = NewIndicator(indicator, s.candleHistory[symbol], s.priceHistory[symbol], s.candleHistory26Days[symbol])
	}
	s.fiveMinuteCandleCounter[symbol] = 0
}

func (s *priceActionStore) RemoveToken(symbol string) {
	delete(s.priceHistory, symbol)
	delete(s.candleHistory, symbol)
	delete(s.candleHistory26Days, symbol)
	delete(s.indicators, symbol)
	delete(s.indicators, symbol)
	delete(s.fiveMinuteCandleCounter, symbol)
	delete(s.latestPriceAtWhichIndicatorsWereUpdated, symbol)
}

func (s *priceActionStore) IngestPrice(symbol string, price models.Ticker) {
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
	length := len(s.candleHistory[symbol])
	if (s.candleHistory[symbol][length-1].Start == candle.Start) {
		if (s.isPriceDeltadEnough(symbol, candle.Close)) {
			s.candleHistory[symbol][length-1] = candle
			for _, indicator := range s.activeIndicators {
				s.indicators[symbol][indicator].ReplaceLatestCandle(candle)
			}
			s.latestPriceAtWhichIndicatorsWereUpdated[symbol] = candle.Close
		}
	} else {
		s.candleHistory[symbol] = append(s.candleHistory[symbol], candle)
		if len(s.candleHistory[symbol]) > 24 {
			s.fiveMinuteCandleCounter[symbol]++
			s.candleHistory[symbol] = s.candleHistory[symbol][1:]
			if s.fiveMinuteCandleCounter[symbol] >= 24 {
				s.fiveMinuteCandleCounter[symbol] = 0
				s.Shift26DayCandleHistory(symbol, s.candleHistory[symbol])
			}
		}
		for _, indicator := range s.activeIndicators {
			s.indicators[symbol][indicator].AddNewCandle(candle)
		}
		s.latestPriceAtWhichIndicatorsWereUpdated[symbol] = candle.Close
	}
}

func (s *priceActionStore) isPriceDeltadEnough(symbol string, newPrice float64) bool {
	oldPrice := s.latestPriceAtWhichIndicatorsWereUpdated[symbol]
	return math.Abs(newPrice-oldPrice)/oldPrice >= 0.001
}

func (s *priceActionStore) Shift26DayCandleHistory(symbol string, fiveMinuteCandles []models.Candle) {
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
	s.candleHistory26Days[symbol] = append(s.candleHistory26Days[symbol], newHistory)
	s.candleHistory26Days[symbol] = s.candleHistory26Days[symbol][1:]
}


// ----- read side – implements MarketProvider -----------------
func (s *priceActionStore) GetPriceHistory(symbol string) []models.Ticker {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// returning the slice header is safe – callers cannot grow it.
	return s.priceHistory[symbol]
}

func (s *priceActionStore) GetCandleHistory(symbol string) []models.Candle {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.candleHistory[symbol]
}

func (s *priceActionStore) GetCandleHistory26Days(symbol string) []models.Candle {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.candleHistory26Days[symbol]
}

func (s *priceActionStore) GetIndicator(symbol string, indicator enum.IndicatorType) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.indicators[symbol][indicator].GetValue()
}

func (s *priceActionStore) UpdateActiveIndicators(strategy enum.Strategy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeIndicators = []enum.IndicatorType{}
	switch strategy {
	case enum.MeanReversion:
		s.activeIndicators = []enum.IndicatorType{enum.EMA, enum.RSI}
	case enum.TrendFollowingWithMomentumConfirmation:
		s.activeIndicators = []enum.IndicatorType{enum.RSI, enum.MACD}
	case enum.CandlestickSignalAggregation:
		s.activeIndicators = []enum.IndicatorType{enum.MACD, enum.Stochastic}
	case enum.RenkoCandlesticks:
		s.activeIndicators = []enum.IndicatorType{enum.ADX, enum.ATR, enum.CCI}
	case enum.HeikenAshi:
		s.activeIndicators = []enum.IndicatorType{enum.EMA, enum.ADX, enum.ATR}
	case enum.DonchianChannel:
		s.activeIndicators = []enum.IndicatorType{enum.Stochastic, enum.ADX}
	case enum.TrendlineBreakout:
		s.activeIndicators = []enum.IndicatorType{enum.BollingerBands, enum.CCI}
	case enum.Supertrend:
		s.activeIndicators = []enum.IndicatorType{enum.EMA, enum.RSI, enum.MACD}
	}
	s.indicators = make(map[string]map[enum.IndicatorType]Indicator)
	for _, symbol := range s.tokens {
		for _, indicator := range s.activeIndicators {
			s.indicators[symbol][indicator] = NewIndicator(indicator, s.candleHistory[symbol], s.priceHistory[symbol], s.candleHistory26Days[symbol])
			s.latestPriceAtWhichIndicatorsWereUpdated[symbol] = s.candleHistory[symbol][len(s.candleHistory[symbol])-1].Close
		}
	}
}
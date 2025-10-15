package helper

import (
	"sync"
	"time"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
)

type IPriceActionStore interface {
	UpdateInboundCandleSize(candleSize enum.CandleSize)
	UpdateCandleSize(symbol string, candleSize enum.CandleSize, longCandleSize enum.CandleSize)
	AddToken(symbol string, candleSize enum.CandleSize, candleHistory []models.Candle, longCandleHistory []models.Candle)
	RemoveToken(symbol string)
	IngestCandleOfInboundCandleSize(candle models.Candle) models.Candle
	GetPriceHistory(symbol string) []models.Ticker
	GetCandleHistory(symbol string) models.CandleHistory
	GetLongCandleHistory(symbol string) models.CandleHistory
}

type PriceActionStore struct {
	mu                      		sync.RWMutex
	tokens                  		[]string
	priceHistory            		map[string][]models.Ticker
	candleHistory           		map[string]*models.CandleHistory
	longCandleHistory       		map[string]*models.CandleHistory
	candleSize              		map[string]enum.CandleSize
	longCandleSize          		map[string]enum.CandleSize
	lastFiveMinuteCandleStart		map[string]time.Time
	storedCandleVolume        		map[string]float64
	volumeOfLastInboundCandle   	map[string]float64
	inboundCandleSize        		enum.CandleSize
}

func NewStore(inboundCandleSize enum.CandleSize) *PriceActionStore {
	store := PriceActionStore{
		tokens:                      []string{},
		priceHistory:                make(map[string][]models.Ticker),
		candleHistory:               make(map[string]*models.CandleHistory),
		longCandleHistory:           make(map[string]*models.CandleHistory),
		candleSize:                  make(map[string]enum.CandleSize),
		longCandleSize:              make(map[string]enum.CandleSize),
		lastFiveMinuteCandleStart:   make(map[string]time.Time),
		storedCandleVolume:        	 make(map[string]float64),
		volumeOfLastInboundCandle:   make(map[string]float64),
	}

	return &store
}

func (s *PriceActionStore) UpdateInboundCandleSize(candleSize enum.CandleSize) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inboundCandleSize = candleSize
}

func (s *PriceActionStore) UpdateCandleSize(symbol string, candleSize enum.CandleSize, candleHistory []models.Candle, longCandleHistory []models.Candle) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.candleSize[symbol] = candleSize
	s.longCandleSize[symbol] = enum.GetLongCandleSizeFromCandleSize(candleSize)
	s.candleHistory[symbol] = models.NewCandleHistory(candleHistory)
	s.longCandleHistory[symbol] = models.NewCandleHistory(longCandleHistory)
	s.lastFiveMinuteCandleStart[symbol] = time.Time{}
	s.storedCandleVolume[symbol] = 0.0
	s.volumeOfLastInboundCandle[symbol] = 0.0
}

func (s *PriceActionStore) AddToken(symbol string, candleSize enum.CandleSize, candleHistory []models.Candle, longCandleHistory []models.Candle) {
	s.mu.Lock()
	s.tokens = append(s.tokens, symbol)
	s.priceHistory[symbol] = make([]models.Ticker, 0)
	s.mu.Unlock()
	s.UpdateCandleSize(symbol, candleSize, candleHistory, longCandleHistory)
}

func (s *PriceActionStore) RemoveToken(symbol string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.priceHistory, symbol)
	delete(s.candleHistory, symbol)
	delete(s.longCandleHistory, symbol)
	delete(s.candleSize, symbol)
	delete(s.longCandleSize, symbol)
	delete(s.storedCandleVolume, symbol)
	delete(s.volumeOfLastInboundCandle, symbol)
	delete(s.lastFiveMinuteCandleStart, symbol)
	idx := 0
	for i, t := range s.tokens {
		if t == symbol {
			idx = i
			break
		}
	}
	s.tokens = append(s.tokens[:idx], s.tokens[idx+1:]...)
}

func (s *PriceActionStore) IngestCandleOfInboundCandleSize(candle models.Candle) models.Candle {
	s.mu.Lock()
	defer s.mu.Unlock()
	symbol := candle.ProductID
	s.ingestPrice(symbol, candle.Close, candle.Start)
	s.updateCandleHistory(symbol, candle, s.candleSize[symbol])
	s.updateCandleHistory(symbol, candle, s.longCandleSize[symbol])
	if (s.lastFiveMinuteCandleStart[symbol].IsZero() || s.lastFiveMinuteCandleStart[symbol].Before(candle.Start)) {
		s.lastFiveMinuteCandleStart[symbol] = candle.Start
	}
	return s.candleHistory[symbol].Candles[len(s.candleHistory[symbol].Candles) - 1]
}

func (s *PriceActionStore) ingestPrice(symbol string, price float64, time time.Time) {
	s.priceHistory[symbol] = append(s.priceHistory[symbol], models.Ticker{
		Symbol:    symbol,
		Price:     price,
		Time:      time,
	})

	length := len(s.priceHistory[symbol])
	if length > 1200 {
		s.priceHistory[symbol] = s.priceHistory[symbol][length - 1200:]
	}
}

func (s *PriceActionStore) GetPriceHistory(symbol string) []models.Ticker {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.priceHistory[symbol]
}

func (s *PriceActionStore) GetCandleHistory(symbol string) models.CandleHistory {
	s.mu.RLock()
	defer s.mu.RUnlock()
	orig := s.candleHistory[symbol]

	merged := make([]models.Candle, 0, len(orig.Candles))
	merged = append(merged, orig.Candles...)

	return models.CandleHistory{Candles: merged}
}

func (s *PriceActionStore) GetLongCandleHistory(symbol string)models.CandleHistory {
	s.mu.RLock()
	defer s.mu.RUnlock()
	orig := s.longCandleHistory[symbol]

	merged := make([]models.Candle, 0, len(orig.Candles))
	merged = append(merged, orig.Candles...)

	return models.CandleHistory{Candles: merged}
}

func (s *PriceActionStore) updateCandleHistory(symbol string, candle models.Candle, candleSize enum.CandleSize) {
	candleHistory := s.candleHistory[symbol]
	timeLastCandle := candleHistory.Candles[len(candleHistory.Candles)-1].Start
	volumeOfCurrentCandle := s.getCurrentCandleVolume(symbol, candle, candleSize)
	if time.Now().Sub(timeLastCandle) > enum.GetTimeDurationFromCandleSize(candleSize) {
		candleHistory.Candles = append(candleHistory.Candles, models.NewCandle(symbol, timeLastCandle, candleSize, candle.Close, volumeOfCurrentCandle))
	} else {
		candleHistory.Candles[len(candleHistory.Candles)-1].UpdateCandle(candle.Close, volumeOfCurrentCandle)
	}
	if len(candleHistory.Candles) > 100 {
		candleHistory.Candles = candleHistory.Candles[1:]
	}
}

func (s *PriceActionStore) getCurrentCandleVolume(symbol string, candle models.Candle, candleSize enum.CandleSize) float64 {
	volumeToSubstract := 0.0
	length := len(s.candleHistory[symbol].Candles)
	if (candleSize == enum.CandleSize5m) {
		return candle.Volume
	} else if (enum.GetTimeDurationFromCandleSize(candleSize) < enum.GetTimeDurationFromCandleSize(enum.CandleSize5m)) {
		numCandles := int(time.Now().Sub(s.lastFiveMinuteCandleStart[symbol]) / enum.GetTimeDurationFromCandleSize(candleSize))
		s.storedCandleVolume[symbol] = 0.0

		for i := int(0); i < numCandles; i++ {
			volumeToSubstract += s.candleHistory[symbol].Candles[length - 1 - i].Volume
		}

		return candle.Volume - volumeToSubstract
	} else {
		timeLastCandle := s.candleHistory[symbol].Candles[len(s.candleHistory[symbol].Candles)-1].Start

		if (time.Now().Sub(timeLastCandle) > enum.GetTimeDurationFromCandleSize(candleSize)) {
			s.storedCandleVolume[symbol] = 0.0
		} else if (s.volumeOfLastInboundCandle[symbol] > candle.Volume) {
			s.storedCandleVolume[symbol] += s.volumeOfLastInboundCandle[symbol]
			s.volumeOfLastInboundCandle[symbol] = candle.Volume
		}

		return s.storedCandleVolume[symbol] + candle.Volume
	}
}

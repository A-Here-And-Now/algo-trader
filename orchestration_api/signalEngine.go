package main

import (
	"context"
	"log"
	"sync"
	"time"
)

// SignalType represents buy/sell; hold is omitted (we don't emit holds)
type SignalType int

const (
	SignalBuy SignalType = iota
	SignalSell
)

func (s SignalType) String() string {
	switch s {
	case SignalBuy:
		return "BUY"
	case SignalSell:
		return "SELL"
	default:
		return "UNKNOWN"
	}
}

type Signal struct {
	Symbol    string
	Type      SignalType
	Percent   float64 // 0-100 meaning percent of allocated funds or position per rules
	Generated time.Time
}

// SignalEngine ingests prices and candles and periodically emits signals
type SignalEngine struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu             sync.RWMutex
	priceFeeds     map[string]chan Ticker // per-token price feed channel (input)
	candleFeeds    map[string]chan Candle    // per-token candle feed channel (input)
	signalChannels map[string]chan Signal    // per-token signal channel (output)

	// in-memory storage per token
	pricesByToken  map[string][]Ticker
	candlesByToken map[string][]Candle
	lastSignalAt   map[string]time.Time
}

func NewSignalEngine(parent context.Context) *SignalEngine {
	ctx, cancel := context.WithCancel(parent)
	return &SignalEngine{
		ctx:            ctx,
		cancel:         cancel,
		priceFeeds:     make(map[string]chan Ticker),
		candleFeeds:    make(map[string]chan Candle),
		signalChannels: make(map[string]chan Signal),
		pricesByToken:  make(map[string][]Ticker),
		candlesByToken: make(map[string][]Candle),
		lastSignalAt:   make(map[string]time.Time),
	}
}

// RegisterToken wires the channels for a token. Manager should create the channels and pass them in.
func (se *SignalEngine) RegisterToken(symbol string, priceCh chan Ticker, candleCh chan Candle, signalCh chan Signal, priceHistory []Ticker, candleHistory []Candle) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.priceFeeds[symbol] = priceCh
	se.candleFeeds[symbol] = candleCh
	se.signalChannels[symbol] = signalCh
	se.pricesByToken[symbol] = priceHistory
	se.candlesByToken[symbol] = candleHistory
}

// UnregisterToken removes channels and state for a token
func (se *SignalEngine) UnregisterToken(symbol string) {
	se.mu.Lock()
	defer se.mu.Unlock()
	delete(se.priceFeeds, symbol)
	delete(se.candleFeeds, symbol)
	delete(se.signalChannels, symbol)
	delete(se.pricesByToken, symbol)
	delete(se.candlesByToken, symbol)
	delete(se.lastSignalAt, symbol)
}

// Start launches the engine goroutine per token
func (se *SignalEngine) Start() {
	go se.run()
}

func (se *SignalEngine) Stop() {
	se.cancel()
}

func (se *SignalEngine) run() {
	// multiplex across dynamic set of channels; we build a snapshot periodically
	// Simple approach: loop with short sleep and non-blocking reads from each channel
	// while also checking timer conditions for signal generation
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-se.ctx.Done():
			return
		case <-ticker.C:
			se.ingestOnceAndMaybeSignal()
		}
	}
}

func (se *SignalEngine) ingestOnceAndMaybeSignal() {
	se.mu.RLock()
	// snapshot keys to avoid holding lock while reading channels
	keys := make([]string, 0, len(se.priceFeeds))
	for k := range se.priceFeeds {
		keys = append(keys, k)
	}
	se.mu.RUnlock()

	for _, symbol := range keys {
		se.mu.RLock()
		priceCh := se.priceFeeds[symbol]
		candleCh := se.candleFeeds[symbol]
		se.mu.RUnlock()

		// drain non-blocking new data
		for drained := 0; drained < 100; drained++ { // cap to avoid infinite loops
			select {
			case tick := <-priceCh:
				se.appendPrice(symbol, tick)
				continue
			case c := <-candleCh:
				se.appendCandle(symbol, c)
				continue
			default:
				// nothing pending
			}
			break
		}

		se.maybeEmitSignal(symbol)
	}
}

func (se *SignalEngine) appendPrice(symbol string, tick Ticker) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.pricesByToken[symbol] = append(se.pricesByToken[symbol], tick)
	// keep only last 10 minutes for memory safety
	cutoff := time.Now().Add(-10 * time.Minute)
	buf := se.pricesByToken[symbol]
	for len(buf) > 0 && buf[0].Timestamp.Before(cutoff) {
		buf = buf[1:]
	}
	se.pricesByToken[symbol] = buf
}

func (se *SignalEngine) appendCandle(symbol string, c Candle) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.candlesByToken[symbol] = append(se.candlesByToken[symbol], c)
	// keep last 200 candles (~hours depending on interval)
	buf := se.candlesByToken[symbol]
	if len(buf) > 200 {
		buf = buf[len(buf)-200:]
	}
	se.candlesByToken[symbol] = buf
}

func (se *SignalEngine) maybeEmitSignal(symbol string) {
	se.mu.RLock()
	lastAt := se.lastSignalAt[symbol]
	prices := se.pricesByToken[symbol]
	candles := se.candlesByToken[symbol]
	signalCh := se.signalChannels[symbol]
	se.mu.RUnlock()

	if signalCh == nil {
		return
	}

	if len(prices) == 0 || len(candles) == 0 {
		return
	}

	// require at least 5 minutes of data and 60s since last signal
	oldest := prices[0].Ts
	if len(candles) > 0 && candles[0].StartTs.Before(oldest) {
		oldest = candles[0].StartTs
	}
	if time.Since(oldest) < 5*time.Minute {
		return
	}
	if !lastAt.IsZero() && time.Since(lastAt) < 60*time.Second {
		return
	}

	// Dummy logic: alternate buy/sell 10% based on last close vs last price
	percent := 10.0
	stype := SignalBuy
	lastPrice := prices[len(prices)-1].Price
	lastClose := candles[len(candles)-1].Close
	if lastPrice < lastClose {
		stype = SignalBuy
	} else if lastPrice > lastClose {
		stype = SignalSell
	} else {
		return
	}

	select {
	case signalCh <- Signal{Symbol: symbol, Type: stype, Percent: percent, Generated: time.Now()}:
		log.Printf("[SignalEngine %s] emitted %s %.2f%%", symbol, stype.String(), percent)
		se.mu.Lock()
		se.lastSignalAt[symbol] = time.Now()
		se.mu.Unlock()
	default:
		// drop if receiver is slow
	}
}

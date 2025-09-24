package main

import (
	"context"
	"log"
	"strconv"
	"sync"
	"time"
)

// SignalEngine ingests prices and candles and periodically emits signals
type SignalEngine struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu                 sync.RWMutex
	signalingResources map[string]*SignalingResource // per-token price feed channel (input)
}

func NewSignalEngine(parent context.Context) *SignalEngine {
	ctx, cancel := context.WithCancel(parent)
	return &SignalEngine{
		ctx:                ctx,
		cancel:             cancel,
		signalingResources: make(map[string]*SignalingResource),
	}
}

// RegisterToken wires the channels for a token. Manager should create the channels and pass them in.
func (se *SignalEngine) RegisterToken(symbol string, traderResource *TraderResource, priceHistory []Ticker, candleHistory []Candle) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.signalingResources[symbol] = NewSignalingResource(traderResource, priceHistory, candleHistory)
}

// UnregisterToken removes channels and state for a token
func (se *SignalEngine) UnregisterToken(symbol string) {
	se.mu.Lock()
	defer se.mu.Unlock()
	delete(se.signalingResources, symbol)
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
	keys := make([]string, 0, len(se.signalingResources))
	for k := range se.signalingResources {
		keys = append(keys, k)
	}
	se.mu.RUnlock()

	for _, symbol := range keys {
		se.mu.RLock()
		priceCh := se.signalingResources[symbol].priceFeed
		candleCh := se.signalingResources[symbol].candleFeed
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
	se.signalingResources[symbol].priceHistory = append(se.signalingResources[symbol].priceHistory, tick)
	// keep only last N ticks for memory safety
	buf := se.signalingResources[symbol].priceHistory
	const maxTicks = 1200 // ~10 minutes if ~2/s
	if len(buf) > maxTicks {
		buf = buf[len(buf)-maxTicks:]
	}
	se.signalingResources[symbol].priceHistory = buf
}

func (se *SignalEngine) appendCandle(symbol string, c Candle) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.signalingResources[symbol].candleHistory = append(se.signalingResources[symbol].candleHistory, c)
	// keep last 200 candles (~hours depending on interval)
	buf := se.signalingResources[symbol].candleHistory
	if len(buf) > 200 {
		buf = buf[len(buf)-200:]
	}
	se.signalingResources[symbol].candleHistory = buf
}

func (se *SignalEngine) maybeEmitSignal(symbol string) {
	se.mu.RLock()
	lastAt := se.signalingResources[symbol].lastSignalAt
	prices := se.signalingResources[symbol].priceHistory
	candles := se.signalingResources[symbol].candleHistory
	signalCh := se.signalingResources[symbol].signalCh
	se.mu.RUnlock()

	if signalCh == nil {
		return
	}

	if len(prices) == 0 || len(candles) == 0 {
		return
	}

	// require at least some data and 60s since last signal
	if len(prices) < 60 || len(candles) < 5 {
		return
	}
	if !lastAt.IsZero() && time.Since(lastAt) < 60*time.Second {
		return
	}

	// Dummy logic: alternate buy/sell 10% based on last close vs last price
	percent := 10.0
	stype := SignalBuy
	lastPriceStr := prices[len(prices)-1].Price
	lastPrice, err := strconv.ParseFloat(lastPriceStr, 64)
	if err != nil {
		return
	}
	lastClose := candles[len(candles)-1].Close
	if lastPrice < lastClose {
		stype = SignalBuy
	} else if lastPrice > lastClose {
		stype = SignalSell
	} else {
		return
	}

	select {
	case signalCh <- Signal{Symbol: symbol, Type: stype, Percent: percent, Time: time.Now()}:
		log.Printf("[SignalEngine %s] emitted %s %.2f%%", symbol, stype.String(), percent)
		se.mu.Lock()
		se.signalingResources[symbol].lastSignalAt = time.Now()
		se.mu.Unlock()
	default:
		// drop if receiver is slow
	}
}

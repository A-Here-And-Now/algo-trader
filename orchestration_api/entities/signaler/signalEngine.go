package signaler

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	exchange "github.com/A-Here-And-Now/algo-trader/orchestration_api/exchange"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
)

// SignalEngine ingests prices and candles and periodically emits signals
type SignalEngine struct {
	ctx              context.Context
	cancel           context.CancelFunc
	exchange         exchange.IExchange
	mu               sync.RWMutex
	lastSignalAt     map[string]time.Time
	tokenStrategies  map[string]Strategy
	tokenCandleSizes map[string]enum.CandleSize
	signalChannels   map[string]chan models.Signal
}

func NewSignalEngine(parent context.Context, strategy enum.Strategy, exchange exchange.IExchange) *SignalEngine {
	ctx, cancel := context.WithCancel(parent)

	se := SignalEngine{
		ctx:              ctx,
		cancel:           cancel,
		exchange:         exchange,
		lastSignalAt:     make(map[string]time.Time),
		tokenStrategies:  make(map[string]Strategy),
		tokenCandleSizes: make(map[string]enum.CandleSize),
		signalChannels:   make(map[string]chan models.Signal),
	}

	return &se
}

func (se *SignalEngine) UpdateStrategy(symbol string, strategy enum.Strategy) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.tokenStrategies[symbol] = NewStrategy(strategy)
}

func (se *SignalEngine) UpdateCandleSize(symbol string, candleSize enum.CandleSize) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.tokenCandleSizes[symbol] = candleSize
}

// RegisterToken wires the channels for a token. Manager should create the channels and pass them in.
func (se *SignalEngine) RegisterToken(symbol string, strategy enum.Strategy, candleSize enum.CandleSize, signalCh chan models.Signal) {
	se.UpdateStrategy(symbol, strategy)
	se.UpdateCandleSize(symbol, candleSize)
	se.mu.Lock()
	se.signalChannels[symbol] = signalCh
	se.lastSignalAt[symbol] = time.Time{}
	se.mu.Unlock()

	go se.run(symbol)
}

// UnregisterToken removes channels and state for a token
func (se *SignalEngine) UnregisterToken(symbol string) {
	se.mu.Lock()
	defer se.mu.Unlock()
	delete(se.signalChannels, symbol)
	delete(se.tokenStrategies, symbol)
	delete(se.tokenCandleSizes, symbol)
	delete(se.lastSignalAt, symbol)
}

func (se *SignalEngine) Stop() {
	se.cancel()
}

func (se *SignalEngine) run(symbol string) {
	for {
		interval := se.getWaitInterval(symbol)
		
		select {
		case <-time.After(interval):
			se.maybeEmitSignal(symbol)
		case <-se.ctx.Done():
			return
		}
	}
}

func (se *SignalEngine) getWaitInterval(symbol string) time.Duration {
	se.mu.RLock()
	candleSize := se.tokenCandleSizes[symbol]
	lastSignal := se.lastSignalAt[symbol]
	se.mu.RUnlock()

	candleDuration := enum.GetTimeDurationFromCandleSize(candleSize)
	
	// If no signal yet, check every 1/25th of candle duration
	if lastSignal.IsZero() {
		return candleDuration / 25
	}
	
	// Calculate when cooldown ends (half candle duration after last signal)
	cooldownEnds := lastSignal.Add(candleDuration / 2)
	remaining := time.Until(cooldownEnds)
	
	// If cooldown has passed, check frequently again
	if remaining <= 0 {
		return candleDuration / 25
	}
	
	// Still in cooldown, wait for it to end
	return remaining
}

func (se *SignalEngine) maybeEmitSignal(symbol string) {
	se.mu.RLock()
	signalCh := se.signalChannels[symbol]
	strategy := se.tokenStrategies[symbol]
	se.mu.RUnlock()

	if se.lastSignalAt[symbol].Before(time.Now().Add(-1 / 2 * enum.GetTimeDurationFromCandleSize(se.tokenCandleSizes[symbol]))) {
		// TODO: should probably be running a unique signaler logic per token spun up in multiple distinct goroutines
		signal := strategy.CalculateSignal(symbol, se.exchange)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		select {
		case signalCh <- signal:
			log.Printf("[SignalEngine %s] emitted %s %.2f%%\n", symbol, signal.Type.String(), signal.Percent)
			strategy.ConfirmSignalDelivered(symbol, signal)
			se.mu.Lock()
			se.lastSignalAt[symbol] = time.Now()
			se.mu.Unlock()
		case <-ticker.C:
			// drop if receiver is slow
		}
	}
}

package signaler

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
)

// SignalEngine ingests prices and candles and periodically emits signals
type SignalEngine struct {
	ctx                context.Context
	cancel             context.CancelFunc
	priceActionStore   *priceActionStore
	mu                 sync.RWMutex
	signalingResources map[string]*SignalingResource // per-token price feed channel (input)
	strategy           Strategy
}

func NewSignalEngine(parent context.Context, strategy enum.Strategy) *SignalEngine {
	ctx, cancel := context.WithCancel(parent)

	se := SignalEngine{
		ctx:                ctx,
		cancel:             cancel,
		priceActionStore:   NewStore(strategy),
		signalingResources: make(map[string]*SignalingResource),
	}

	se.UpdateStrategy(strategy)

	return &se
}

func (se *SignalEngine) UpdateStrategy(strategy enum.Strategy) {
	se.strategy = NewStrategy(strategy)
}

// RegisterToken wires the channels for a token. Manager should create the channels and pass them in.
func (se *SignalEngine) RegisterToken(symbol string, priceFeed chan models.Ticker, candleFeed chan models.Candle, signalCh chan models.Signal, priceHistory []models.Ticker, candleHistory []models.Candle, candleHistory26Days []models.Candle) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.priceActionStore.AddToken(symbol, priceHistory, candleHistory, candleHistory26Days)
	se.signalingResources[symbol] = NewSignalingResource(priceFeed, candleFeed, signalCh, priceHistory, candleHistory, candleHistory26Days)
}

// UnregisterToken removes channels and state for a token
func (se *SignalEngine) UnregisterToken(symbol string) {
	se.mu.Lock()
	defer se.mu.Unlock()
	delete(se.signalingResources, symbol)
	se.priceActionStore.RemoveToken(symbol)
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
		drain:
		for drained := 0; drained < 10; drained++ { // cap to avoid infinite loops
			select {
			case t := <-priceCh:
				se.priceActionStore.IngestPrice(symbol, t)
				continue
			case c := <-candleCh:
				se.priceActionStore.IngestCandle(symbol, c)
				continue
			default:
				break drain // break out since no data
			}
		}

		se.maybeEmitSignal(symbol)
	}
}

func (se *SignalEngine) maybeEmitSignal(symbol string) {
	se.mu.RLock()
	lastAt := se.signalingResources[symbol].lastSignalAt
	signalCh := se.signalingResources[symbol].signalCh
	se.mu.RUnlock()

	if lastAt.Before(time.Now().Add(-1 * time.Minute)) {
		signal := se.strategy.CalculateSignal(symbol, se.priceActionStore)
		select {
		case signalCh <- signal:
			log.Printf("[SignalEngine %s] emitted %s %.2f%%\n", symbol, signal.Type.String(), signal.Percent)
			se.strategy.ConfirmSignalDelivered(symbol, signal.Type)
			se.mu.Lock()
			se.signalingResources[symbol].lastSignalAt = time.Now()
			se.mu.Unlock()
		default:
			// drop if receiver is slow
		}
	}
}

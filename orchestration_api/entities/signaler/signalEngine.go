package signaler

import (
	"context"
	"log"
	"sync"
	"time"

	strategy_helper "github.com/A-Here-And-Now/algo-trader/orchestration_api/entities/signaler/strategy_helper"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
)

// SignalEngine ingests prices and candles and periodically emits signals
type SignalEngine struct {
	ctx                context.Context
	cancel             context.CancelFunc
	priceActionStore   *strategy_helper.PriceActionStore
	mu                 sync.RWMutex
	signalingResources map[string]*SignalingResource // per-token price feed channel (input)
	strategy           Strategy
}

func NewSignalEngine(parent context.Context, strategy enum.Strategy) *SignalEngine {
	ctx, cancel := context.WithCancel(parent)

	se := SignalEngine{
		ctx:                ctx,
		cancel:             cancel,
		priceActionStore:   strategy_helper.NewStore(strategy),
		signalingResources: make(map[string]*SignalingResource),
	}

	se.UpdateStrategy(strategy)

	return &se
}

func (se *SignalEngine) UpdateStrategy(strategy enum.Strategy) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.priceActionStore.UpdateStrategy(strategy)
	se.strategy = NewStrategy(strategy)
}

// RegisterToken wires the channels for a token. Manager should create the channels and pass them in.
func (se *SignalEngine) RegisterToken(symbol string, priceFeed chan models.Ticker, candleFeed chan models.Candle, signalCh chan models.Signal, priceHistory []models.Ticker, candleHistory []models.Candle, candleHistory26Days []models.Candle) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.priceActionStore.AddToken(symbol, priceHistory, candleHistory, candleHistory26Days)
	se.signalingResources[symbol] = NewSignalingResource(priceFeed, candleFeed, signalCh, priceHistory, candleHistory, candleHistory26Days)
	go se.run(symbol)
}

// UnregisterToken removes channels and state for a token
func (se *SignalEngine) UnregisterToken(symbol string) {
	se.mu.Lock()
	defer se.mu.Unlock()
	delete(se.signalingResources, symbol)
	se.priceActionStore.RemoveToken(symbol)
}

func (se *SignalEngine) Stop() {
	se.cancel()
}

func (se *SignalEngine) run(symbol string) {
	for {
		select {
		case <-se.ctx.Done():
			return
		default:
			se.ingestOnceAndMaybeSignal(symbol)
		}
	}
}

func (se *SignalEngine) ingestOnceAndMaybeSignal(symbol string) {
	se.mu.RLock()
	priceCh := se.signalingResources[symbol].priceFeed
	candleCh := se.signalingResources[symbol].candleFeed
	priceActionStore := se.priceActionStore
	se.mu.RUnlock()

	// drain non-blocking new data
	drain: for drained := 0; drained < 10; drained++ { // cap to avoid infinite loops
		select {
		case t := <-priceCh:
			priceActionStore.IngestPrice(symbol, t)
			if (se.strategy != nil){
				se.strategy.UpdateTrailingStop(symbol, t)
			}
			continue
		case c := <-candleCh:
			priceActionStore.IngestCandle(symbol, c)
			continue
		default:
			break drain // break out since no data
		}
	}

	se.maybeEmitSignal(symbol)
}

func (se *SignalEngine) maybeEmitSignal(symbol string) {
	se.mu.RLock()
	signalingResource := se.signalingResources[symbol]
	lastAt := signalingResource.lastSignalAt
	signalCh := signalingResource.signalCh
	strategy := se.strategy
	priceActionStore := se.priceActionStore
	se.mu.RUnlock()

	if lastAt.Before(time.Now().Add(-1 * time.Minute)) {
		// TODO: should probably be running a unique signaler logic per token spun up in multiple distinct goroutines
		signal := strategy.CalculateSignal(symbol, priceActionStore)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()	
		select {
		case signalCh <- signal:
			log.Printf("[SignalEngine %s] emitted %s %.2f%%\n", symbol, signal.Type.String(), signal.Percent)
			strategy.ConfirmSignalDelivered(symbol, signal)
			se.mu.Lock()
			se.signalingResources[symbol].lastSignalAt = time.Now()
			se.mu.Unlock()
		case <-ticker.C:
			// drop if receiver is slow
		}
	}
}

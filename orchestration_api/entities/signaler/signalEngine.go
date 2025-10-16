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

type SignalEngineConfigUpdate struct {
	Symbol     string
	Strategy   enum.Strategy
	CandleSize enum.CandleSize
}

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
	tickerChannels   map[string]<-chan models.Ticker
	tickerCleanup    map[string]func()
	tokenEnabled     map[string]bool
	updateCh         <-chan SignalEngineConfigUpdate
}

func NewSignalEngine(parent context.Context, exchange exchange.IExchange, updateCh <-chan SignalEngineConfigUpdate) *SignalEngine {
	ctx, cancel := context.WithCancel(parent)

	se := SignalEngine{
		ctx:              ctx,
		cancel:           cancel,
		exchange:         exchange,
		lastSignalAt:     make(map[string]time.Time),
		tokenStrategies:  make(map[string]Strategy),
		tokenCandleSizes: make(map[string]enum.CandleSize),
		signalChannels:   make(map[string]chan models.Signal),
		tickerChannels:   make(map[string]<-chan models.Ticker),
		tickerCleanup:    make(map[string]func()),
		tokenEnabled:     make(map[string]bool),
		updateCh:         updateCh,
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
	tickerCh, tickerCleanup := se.exchange.SubscribeToTicker(symbol)
	se.UpdateStrategy(symbol, strategy)
	se.UpdateCandleSize(symbol, candleSize)
	se.mu.Lock()
	se.tickerChannels[symbol] = tickerCh
	se.tickerCleanup[symbol] = tickerCleanup
	se.signalChannels[symbol] = signalCh
	se.lastSignalAt[symbol] = time.Time{}
	se.tokenEnabled[symbol] = true
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
	se.tickerCleanup[symbol]()
	delete(se.tickerChannels, symbol)
	delete(se.tickerCleanup, symbol)
	delete(se.tokenEnabled, symbol)
}

func (se *SignalEngine) Stop() {
	se.cancel()
}

func (se *SignalEngine) run(symbol string) {
	for {
		interval := se.getWaitInterval(symbol)

		select {
		case <-se.ctx.Done():
			return
		case update, ok := <-se.updateCh:
			if se.tokenIsDisabled(symbol) || !ok {
				return
			}
			se.UpdateStrategy(update.Symbol, update.Strategy)
			se.UpdateCandleSize(update.Symbol, update.CandleSize)
		case <-time.After(interval):
			if se.tokenIsDisabled(symbol) {
				return
			}
			se.emitSignal(symbol)
		case ticker, ok := <-se.tickerChannels[symbol]:
			if se.tokenIsDisabled(symbol) || !ok {
				return
			}
			se.tokenStrategies[symbol].UpdateTrailingStop(symbol, ticker)
		}
	}
}

func (se *SignalEngine) getWaitInterval(symbol string) time.Duration {
	se.mu.Lock()
	candleSize := se.tokenCandleSizes[symbol]
	lastSignal := se.lastSignalAt[symbol]
	se.mu.Unlock()

	candleDuration := enum.GetTimeDurationFromCandleSize(candleSize)

	// If no signal yet, check every 1/25th of candle duration
	if lastSignal.IsZero() {
		waitInterval := candleDuration / 25
		if waitInterval < 15*time.Second {
			return 15 * time.Second
		}
		return waitInterval
	}

	// Calculate when cooldown ends (half candle duration after last signal)
	cooldownEnds := lastSignal.Add(candleDuration / 2)
	remaining := time.Until(cooldownEnds)

	// If cooldown has passed, check frequently again
	if remaining <= 0 {
		waitInterval := candleDuration / 25
		if waitInterval < 15*time.Second {
			return 15 * time.Second
		}
		return waitInterval
	}

	// Still in cooldown, wait for it to end
	return remaining
}

func (se *SignalEngine) emitSignal(symbol string) {
	se.mu.Lock()
	signalCh := se.signalChannels[symbol]
	strategy := se.tokenStrategies[symbol]

	se.mu.Unlock()

	signal := strategy.CalculateSignal(symbol, se.exchange)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	if se.tokenIsDisabled(symbol) {
		return
	}
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

func (se *SignalEngine) tokenIsDisabled(symbol string) bool {
	se.mu.Lock()
	defer se.mu.Unlock()
	if _, present := se.tokenEnabled[symbol]; !present {
		return true
	}
	if se.ctx.Err() != nil {
		return true
	}
	return false
}
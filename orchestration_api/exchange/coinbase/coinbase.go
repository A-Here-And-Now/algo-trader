package coinbase

import (
	"context"
	"fmt"
	"log"
	"net/url"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/coinbase"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
	"github.com/gorilla/websocket"
	"sync"
	exchange_helper "github.com/A-Here-And-Now/algo-trader/orchestration_api/exchange/helper"
)

// CoinbaseExchange implements the Exchange interface for Coinbase Advanced Trade API
type CoinbaseExchange struct {
	mu           sync.RWMutex
	ctx          context.Context
	marketDataWS *websocket.Conn
	userDataWS   *websocket.Conn
	apiKey       string
	apiSecret    string

	// Track subscriptions per symbol and candle size
	symbolSubscriptions map[string]bool

	// Subscription channels per symbol and candle size
	candleChannels map[string][]chan models.Candle
	candleSizePerSymbol map[string]enum.CandleSize
	inboundCandleSize enum.CandleSize
	// Ticker channels per symbol
	tickerChannels map[string][]chan models.Ticker

	// Order update channels per symbol
	orderChannels map[string][]chan models.OrderUpdate

	client              *coinbase.CoinbaseClient
	priceActionStore    *exchange_helper.PriceActionStore
}

func NewCoinbaseExchange(ctx context.Context, apiKey, apiSecret string, inboundCandleSize enum.CandleSize) *CoinbaseExchange {
	return &CoinbaseExchange{
		ctx:                 ctx,
		apiKey:              apiKey,
		apiSecret:           apiSecret,
		symbolSubscriptions: make(map[string]bool),
		candleChannels:      make(map[string][]chan models.Candle),
		tickerChannels:      make(map[string][]chan models.Ticker),
		orderChannels:       make(map[string][]chan models.OrderUpdate),
		client:              coinbase.NewCoinbaseClient("https://api.coinbase.com", apiKey, apiSecret),
		priceActionStore:    exchange_helper.NewStore(inboundCandleSize),
	}
}

func (e *CoinbaseExchange) SubscribeToOrderUpdates(symbol string) (<-chan models.OrderUpdate, func(), error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	ch := make(chan models.OrderUpdate, 10)

	if e.orderChannels[symbol] == nil {
		e.orderChannels[symbol] = make([]chan models.OrderUpdate, 0)
	}
	e.orderChannels[symbol] = append(e.orderChannels[symbol], ch)

	cleanup := func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		close(ch)
		// Remove channel from slice
		channels := e.orderChannels[symbol]
		for i, c := range channels {
			if c == ch {
				e.orderChannels[symbol] = append(channels[:i], channels[i+1:]...)
				break
			}
		}
	}

	return ch, cleanup, nil
}

func (e *CoinbaseExchange) SubscribeToTicker(symbol string) (<-chan models.Ticker, func(), error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	ch := make(chan models.Ticker, 10)

	if e.tickerChannels[symbol] == nil {
		e.tickerChannels[symbol] = make([]chan models.Ticker, 0)
	}
	e.tickerChannels[symbol] = append(e.tickerChannels[symbol], ch)

	cleanup := func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		close(ch)
		// Remove channel from slice
		channels := e.tickerChannels[symbol]
		for i, c := range channels {
			if c == ch {
				e.tickerChannels[symbol] = append(channels[:i], channels[i+1:]...)
				break
			}
		}
	}

	return ch, cleanup, nil
}

func (e *CoinbaseExchange) SubscribeToCandle(symbol string) (<-chan models.Candle, func(), error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	ch := make(chan models.Candle, 10)

	if e.candleChannels[symbol] == nil {
		e.candleChannels[symbol] = make([]chan models.Candle, 0)
	}
	e.candleChannels[symbol] = append(e.candleChannels[symbol], ch)

	cleanup := func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		close(ch)
		// Remove channel from slice
		if channels, ok := e.candleChannels[symbol]; ok {
			for i, c := range channels {
				if c == ch {
					e.candleChannels[symbol] = append(channels[:i], channels[i+1:]...)
					break
				}
			}
		}
	}

	return ch, cleanup, nil
}

func (e *CoinbaseExchange) GetCandleHistory(symbol string) models.CandleHistory {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.priceActionStore.GetCandleHistory(symbol)
}

func (e *CoinbaseExchange) GetLongCandleHistory(symbol string) models.CandleHistory {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.priceActionStore.GetLongCandleHistory(symbol)
}

func (e *CoinbaseExchange) GetPriceHistory(symbol string) []models.Ticker {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.priceActionStore.GetPriceHistory(symbol)
}

func (e *CoinbaseExchange) UpdateInboundCandleSize(candleSize enum.CandleSize) {
	e.priceActionStore.UpdateInboundCandleSize(candleSize)
}

func (e *CoinbaseExchange) StartNewTokenDataStream(symbol string, candleSize enum.CandleSize) error {
	historicalCandles, longHistoricalCandles, err := e.getHistoricalCandleSets(symbol, candleSize)
	if err != nil {
		log.Printf("%v", err)
		return err
	}
	e.priceActionStore.AddToken(symbol, candleSize, historicalCandles, longHistoricalCandles)

	e.mu.Lock()
	e.symbolSubscriptions[symbol] = true
	marketDataWS := e.marketDataWS
	userDataWS := e.userDataWS
	e.mu.Unlock()
	marketDataSubPayload := coinbase.GetMarketSubscriptionPayload([]string{symbol}, false)

	if marketDataWS != nil {
		for _, p := range marketDataSubPayload {
			if err := marketDataWS.WriteJSON(p); err != nil {
				log.Printf("Failed to send subscription: %v", err)
				return err
			}
		}
	}

	if userDataWS != nil {
		userDataSubPayload, err := e.getUserDataSubscriptionPayload([]string{symbol}, false)
		if err != nil {
			log.Printf("failed to get user data subscription payload: %v", err)
			return err
		}
		if err := userDataWS.WriteJSON(userDataSubPayload); err != nil {
			log.Printf("Failed to send subscription: %v", err)
			return err
		}
	}
	
	return nil
}

func (e *CoinbaseExchange) StopTokenDataStream(symbol string) error {
	e.mu.Lock()
	e.symbolSubscriptions[symbol] = false
	marketDataWS := e.marketDataWS
	userDataWS := e.userDataWS
	e.mu.Unlock()
	e.priceActionStore.RemoveToken(symbol)
	marketDataSubPayload := coinbase.GetMarketSubscriptionPayload([]string{symbol}, true)

	if marketDataWS != nil {
		for _, p := range marketDataSubPayload {
			if err := marketDataWS.WriteJSON(p); err != nil {
				log.Printf("Failed to send unsubscription: %v", err)
				return err
			}
		}
	}

	if userDataWS != nil {
		userDataSubPayload, err := e.getUserDataSubscriptionPayload([]string{symbol}, true)
		if err != nil {
			log.Printf("failed to get user data unsubscription payload: %v", err)
			return err
		}
		if err := userDataWS.WriteJSON(userDataSubPayload); err != nil {
			log.Printf("Failed to send unsubscription: %v", err)
			return err
		}
	}

	return nil
}

func (e *CoinbaseExchange) UpdateCandleSizeForSymbol(symbol string, candleSize enum.CandleSize) error {
	historicalCandles, longHistoricalCandles, err := e.getHistoricalCandleSets(symbol, candleSize)
	if err != nil {
		log.Printf("%v", err)
		return err
	}

	e.priceActionStore.UpdateCandleSize(symbol, candleSize, historicalCandles, longHistoricalCandles)
	return nil
}

func (e *CoinbaseExchange) StartOrderAndPositionValuationWebSocket(ctx context.Context, wsURL string) {
	go e.runUserWebSocket(ctx, wsURL)
}

func (e *CoinbaseExchange) StartCoinbaseFeed(ctx context.Context, cbAdvUrl string) {
	u, _ := url.Parse(cbAdvUrl)
	go e.runMarketDataWebSocket(ctx, u.String())
}

func (e *CoinbaseExchange) getHistoricalCandleSets(symbol string, candleSize enum.CandleSize) ([]models.Candle, []models.Candle, error) {
	historicalCandles, err1 := e.client.GetHistoricalCandles(e.ctx, symbol, candleSize)
	longHistoricalCandles, err2 := e.client.GetHistoricalCandles(e.ctx, symbol, enum.GetLongCandleSizeFromCandleSize(candleSize))
	if err1 != nil || err2 != nil {
		err := fmt.Errorf("failed to get historical candles: %v, %v", err1, err2)
		log.Printf("%v", err)
		return nil, nil, err
	}
	return models.GetDomainCandlesFromHistoricalCandles(symbol, historicalCandles.Candles), models.GetDomainCandlesFromHistoricalCandles(symbol, longHistoricalCandles.Candles), nil
}
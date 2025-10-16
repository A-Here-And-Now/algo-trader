package coinbase

import (
	"context"
	"fmt"
	"log"
	"net/url"

	"sync"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	exchange_helper "github.com/A-Here-And-Now/algo-trader/orchestration_api/exchange/helper"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
	cb_models "github.com/A-Here-And-Now/algo-trader/orchestration_api/models/coinbase"
	"github.com/gorilla/websocket"
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

	// Ticker channels per symbol
	tickerChannels map[string][]chan models.Ticker

	// Order update channels per symbol
	orderChannels map[string][]chan models.OrderUpdate

	client           *CoinbaseClient
	priceActionStore *exchange_helper.PriceActionStore
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
		client:              newCoinbaseClient("https://api.coinbase.com", apiKey, apiSecret),
		priceActionStore:    exchange_helper.NewStore(inboundCandleSize),
	}
}

func (e *CoinbaseExchange) SubscribeToOrderUpdates(symbol string) (<-chan models.OrderUpdate, func()) {
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

		if channels, ok := e.orderChannels[symbol]; ok {
			for i, c := range channels {
				if c == ch {
					close(ch)
					e.orderChannels[symbol] = append(channels[:i], channels[i+1:]...)
					break
				}
			}
		}
	}

	return ch, cleanup
}

func (e *CoinbaseExchange) SubscribeToTicker(symbol string) (<-chan models.Ticker, func()) {
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

		channels := e.tickerChannels[symbol]
		for i, c := range channels {
			if c == ch {
				close(ch)
				e.tickerChannels[symbol] = append(channels[:i], channels[i+1:]...)
				break
			}
		}
	}

	return ch, cleanup
}

func (e *CoinbaseExchange) SubscribeToCandle(symbol string) (<-chan models.Candle, func()) {
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

		if channels, ok := e.candleChannels[symbol]; ok {
			for i, c := range channels {
				if c == ch {
					close(ch)
					e.candleChannels[symbol] = append(channels[:i], channels[i+1:]...)
					break
				}
			}
		}
	}

	return ch, cleanup
}

func (e *CoinbaseExchange) GetCandleHistory(symbol string) models.CandleHistory {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.priceActionStore.GetCandleHistory(symbol)
}

func (e *CoinbaseExchange) GetLongCandleHistory(symbol string) models.CandleHistory {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.priceActionStore.GetLongCandleHistory(symbol)
}

func (e *CoinbaseExchange) GetPriceHistory(symbol string) []models.Ticker {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.priceActionStore.GetPriceHistory(symbol)
}

func (e *CoinbaseExchange) UpdateInboundCandleSize(candleSize enum.CandleSize) {
	e.priceActionStore.UpdateInboundCandleSize(candleSize)
}

func (e *CoinbaseExchange) StartNewTokenDataStream(symbol string, candleSize enum.CandleSize) error {
	if e.symbolSubscriptions[symbol] {
		return nil
	}
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
	marketDataSubPayload := cb_models.GetMarketSubscriptionPayload([]string{symbol}, false)

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
	// Close all channels for this symbol
	if channels, ok := e.candleChannels[symbol]; ok {
		for _, ch := range channels {
			close(ch)
		}
		delete(e.candleChannels, symbol)
	}

	if channels, ok := e.tickerChannels[symbol]; ok {
		for _, ch := range channels {
			close(ch)
		}
		delete(e.tickerChannels, symbol)
	}

	if channels, ok := e.orderChannels[symbol]; ok {
		for _, ch := range channels {
			close(ch)
		}
		delete(e.orderChannels, symbol)
	}

	marketDataWS := e.marketDataWS
	userDataWS := e.userDataWS
	e.mu.Unlock()
	e.priceActionStore.RemoveToken(symbol)
	marketDataSubPayload := cb_models.GetMarketSubscriptionPayload([]string{symbol}, true)

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

	e.clearSymbolSubscriptions(symbol)
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

func (e *CoinbaseExchange) GetHistoricalCandles(ctx context.Context, productID string, candleSize enum.CandleSize) (cb_models.CandlesResponse, error) {
	return e.client.GetHistoricalCandles(ctx, productID, candleSize)
}

func (e *CoinbaseExchange) ListAccounts(ctx context.Context) (cb_models.AccountsListResponse, error) {
	return e.client.ListAccounts(ctx)
}

func (e *CoinbaseExchange) GetAllTokenBalances(ctx context.Context) (map[string]float64, error) {
	return e.client.GetAllTokenBalances(ctx)
}

func (e *CoinbaseExchange) ListOrders(ctx context.Context, productID string, limit int) (cb_models.ListOrdersResponse, error) {
	return e.client.ListOrders(ctx, productID, limit)
}

func (e *CoinbaseExchange) CreateOrder(ctx context.Context, productID string, amountOfUSD float64, isBuy bool) (cb_models.CreateOrderResponse, error) {
	return e.client.CreateOrder(ctx, productID, amountOfUSD, isBuy)
}

func (e *CoinbaseExchange) SellTokens(ctx context.Context, productID string, amountOfUSD float64) (cb_models.CreateOrderResponse, error) {
	return e.client.SellTokens(ctx, productID, amountOfUSD)
}

func (e *CoinbaseExchange) EditOrder(ctx context.Context, body []byte) (cb_models.EditOrderResponse, error) {
	return e.client.EditOrder(ctx, body)
}

func (e *CoinbaseExchange) CancelOrders(ctx context.Context, orderID string) error {
	return e.client.CancelOrders(ctx, orderID)
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

func (e *CoinbaseExchange) clearSymbolSubscriptions(symbol string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Just delete - let the cleanup functions handle closing
	delete(e.candleChannels, symbol)
	delete(e.tickerChannels, symbol)
	delete(e.orderChannels, symbol)
}

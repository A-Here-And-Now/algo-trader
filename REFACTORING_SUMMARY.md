# Exchange Refactoring Summary

## Overview
Refactored the websocket operations from the Manager entity to a dedicated Exchange interface, enabling traders and signalers to interact directly with the exchange for market data and order updates.

## Key Changes

### 1. New Files Created

#### `orchestration_api/entities/exchange/marketdata/coinbase/exchange.go`
- Created `CoinbaseExchange` struct that implements the Exchange interface
- Tracks candle history per symbol **AND** per candle size (key improvement)
- Manages subscriptions for tickers, candles, and order updates
- Provides frontend feed channels for Manager to use
- Thread-safe operations with mutex protection

**Key Features:**
- `candleHistories`: `map[string]map[enum.CandleSize]*models.CandleHistory` - supports multiple candle sizes per symbol
- Subscription methods: `SubscribeToTicker()`, `SubscribeToCandle()`, `SubscribeToOrderUpdates()`
- History retrieval: `GetCandleHistory()`, `GetPriceHistory()`, `GetHeikenAshiCandleHistory()`, `GetRenkoCandleHistory()`
- Frontend channels: `GetFrontendCandleFeed()`, `GetFrontendPriceFeed()`, `GetFrontendOrderFeed()`

#### `orchestration_api/entities/exchange/marketdata/coinbase/helpers.go`
- Helper function `GetOrderUpdate()` to convert Coinbase Orders to OrderUpdate models

#### `orchestration_api/entities/manager/websocketHandler.go`
- Extracted frontend WebSocket handler from websocketOps.go
- Updated to use Exchange interface for data retrieval
- Manager now acts as a frontend data aggregator rather than middleman

### 2. Refactored Files

#### `orchestration_api/entities/exchange/marketdata/coinbase/websocketOps.go`
**Before:** Methods on `Manager` struct
**After:** Methods on `CoinbaseExchange` struct

**Refactored Methods:**
- `StartCoinbaseFeed()` - starts market data websocket
- `runMarketDataWebSocket()` - manages market data connection with reconnection logic
- `readLoop()` - reads and routes ticker/candle data
- `writeCandle()` - writes candles to all subscribers (multiple candle sizes supported)
- `writePrice()` - writes tickers to all subscribers
- `subscribeToMarketDataForAllTokens()` - subscribes to market data
- `subscribeToNewToken()` - adds new token subscription
- `StartOrderAndPositionValuationWebSocket()` - starts user data websocket
- `runUserWebSocket()` - manages user data connection
- `readUserLoop()` - reads order updates
- `routeOrderUpdate()` - routes order updates to subscribers

**Key Improvements:**
- Thread-safe access to websocket connections
- Support for multiple concurrent subscriptions per symbol
- Candle history now tracked per candle size, not just one size per symbol

#### `orchestration_api/entities/manager/manager.go`
**Removed:**
- `marketDataWS`, `userDataWS` fields (moved to Exchange)
- `marketPriceResources` field (functionality moved to Exchange)
- `subscriptionChannel` field (moved to Exchange)
- `safeAddMarketPriceResource()`, `safeRemoveMarketPriceResource()` methods
- `safeGetMarketPriceResources()` method

**Added:**
- `exchange Exchange` field - pointer to Exchange interface
- `Exchange` interface definition in manager package

**Updated:**
- `NewManager()` - now accepts `Exchange` parameter
- `Start()` - gets initial data from exchange, traders/signalers interact with exchange directly
- `GetAllPriceHistory()`, `GetAllCandleHistory()` - now query exchange instead of marketPriceResources

## Architecture Improvements

### Before
```
Frontend <-> Manager <-> Coinbase WebSockets
                 |
                 v
          Traders/Signalers
```

### After
```
Frontend <-> Manager -> Exchange <-> Coinbase WebSockets
                            ^
                            |
                     Traders/Signalers
```

**Benefits:**
1. **Decoupling**: Manager no longer acts as middleman for market data
2. **Direct Access**: Traders and Signalers can subscribe directly to Exchange
3. **Scalability**: Exchange handles multiple subscriptions per symbol efficiently
4. **Flexibility**: Easy to add new exchanges (e.g., Uniswap) by implementing Exchange interface
5. **Multiple Candle Sizes**: Each symbol can now track multiple candle sizes simultaneously

## TODO Items for Future Work

### 1. Candle Size Detection
Currently defaulting to 5m candles. Need to:
- Parse candle size from incoming candle data
- Make candle size configurable per trader/signaler

### 2. Historical Candle Data
The 26-day candle history initialization needs to be implemented:
```go
// In Manager.Start()
// TODO: Get 26-day candle history for initial strategy state
candleHistory26Days := candleHistory.Candles // Placeholder for now
```

### 3. Update Trader and Signaler
Modify trader and signaler goroutines to:
- Accept Exchange pointer in their constructors
- Subscribe directly to Exchange for their data needs
- Remove dependency on channels passed from Manager

### 4. Frontend Subscription Tracking
The WebSocketHandler needs to properly track subscribed symbols:
```go
// Currently using empty map
subscribedSymbols := make(map[string]bool)
// Need to populate this when user subscribes
```

### 5. Timestamp Parsing
The `GetOrderUpdate()` helper has a commented out timestamp field:
```go
// Note: Ts field needs to be parsed from CreationTime string
// Ts: o.CreationTime,
```
Need to parse the CreationTime string to time.Time

### 6. Exchange Initialization
The orchestration.go (or main entry point) needs to:
- Create a CoinbaseExchange instance
- Pass it to NewManager()
- Start the exchange websockets

Example:
```go
exchange := coinbase.NewCoinbaseExchange(ctx, apiKey, apiSecret)
exchange.StartCoinbaseFeed(ctx, "wss://advanced-trade-ws.coinbase.com")
exchange.StartOrderAndPositionValuationWebSocket(ctx, "wss://advanced-trade-ws.coinbase.com")

manager := manager.NewManager(funds, maxPL, strategy, ctx, apiKey, apiSecret, tokens, exchange)
```

## Testing Recommendations

1. **Unit Tests**: Test Exchange subscription mechanisms
2. **Integration Tests**: Test Manager -> Exchange -> Traders/Signalers flow
3. **Concurrency Tests**: Verify thread-safety of Exchange operations
4. **Multiple Candle Sizes**: Test that different candle sizes can be tracked simultaneously
5. **Reconnection Logic**: Test websocket reconnection behavior

## Migration Notes

### Breaking Changes
- `NewManager()` signature changed - now requires `Exchange` parameter
- `marketPriceResources` no longer accessible from Manager
- Websocket methods removed from Manager (now in Exchange)

### Backward Compatibility
- API endpoints should remain unchanged
- Frontend WebSocket protocol unchanged
- Trader/Signaler interfaces remain same (for now, until they're updated to use Exchange directly)

## Performance Considerations

1. **Memory**: Each symbol now can have multiple candle history maps (one per candle size)
2. **Channels**: More channels created for subscriptions (one per subscriber per symbol)
3. **Locking**: Mutex locking in Exchange - optimized with RLock for reads

## Security Considerations

- API keys stored in Exchange (same as before in Manager)
- JWT generation for user websocket subscriptions
- Thread-safe access to shared resources


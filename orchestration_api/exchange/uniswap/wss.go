//go:build uniswap

package uniswap

import (
	"context"
	"errors"
	"log"
	"math"
	"math/big"
	"strconv"
	"sync"
	"time"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/entities/marketdata"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Config holds RPC endpoints and pool settings for Uniswap market data.
type Config struct {
	WSSURL string
	// Map from symbol like "ETH-USDC" to pool address
	SymbolToPool map[string]string
	// Decimals for token0/token1 per symbol (needed to compute prices)
	SymbolToDecimals map[string]struct{ Dec0, Dec1 int }
	// Optional: topic0 of the Swap event. If empty, we subscribe by address only.
	SwapTopic string
}

type Provider struct {
	cli    *ethclient.Client
	cfg    Config
	cancel context.CancelFunc
	mu     sync.Mutex
	closed bool
}

func NewProvider(ctx context.Context, cfg Config) (*Provider, error) {
	cli, err := ethclient.DialContext(ctx, cfg.WSSURL)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	return &Provider{cli: cli, cfg: cfg, cancel: cancel}, nil
}

func (p *Provider) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	if p.cancel != nil {
		p.cancel()
	}
}

// SubscribeTicker implements marketdata.MarketData.
func (p *Provider) SubscribeTicker(symbol string) (<-chan models.Ticker, func(), error) {
	poolAddrHex, ok := p.cfg.SymbolToPool[symbol]
	if !ok {
		return nil, nil, errors.New("unknown symbol: " + symbol)
	}
	poolAddr := common.HexToAddress(poolAddrHex)

	out := make(chan models.Ticker, 64)
	ctx, cancel := context.WithCancel(context.Background())

	go p.streamSwaps(ctx, poolAddr, symbol, out, nil)

	return out, func() {
		cancel()
	}, nil
}

// SubscribeCandles implements marketdata.MarketData using an in-memory bucket aggregator.
func (p *Provider) SubscribeCandles(symbol string, tf marketdata.CandleTF) (<-chan models.Candle, func(), error) {
	poolAddrHex, ok := p.cfg.SymbolToPool[symbol]
	if !ok {
		return nil, nil, errors.New("unknown symbol: " + symbol)
	}
	poolAddr := common.HexToAddress(poolAddrHex)

	dec, _ := p.cfg.SymbolToDecimals[symbol]

	out := make(chan models.Candle, 32)
	ctx, cancel := context.WithCancel(context.Background())

	interval := tf.Duration()
	go p.streamSwaps(ctx, poolAddr, symbol, nil, &candleAgg{
		out:      out,
		interval: interval,
		symbol:   symbol,
		dec0:     dec.Dec0,
		dec1:     dec.Dec1,
	})

	return out, func() {
		cancel()
	}, nil
}

// streamSwaps subscribes to pool Swap logs, computing price and dispatching ticker/candles.
func (p *Provider) streamSwaps(ctx context.Context, pool common.Address, symbol string, tickCh chan<- models.Ticker, agg *candleAgg) {
	q := ethereum.FilterQuery{
		Addresses: []common.Address{pool},
	}
	if p.cfg.SwapTopic != "" {
		q.Topics = [][]common.Hash{{common.HexToHash(p.cfg.SwapTopic)}}
	}
	logsCh := make(chan types.Log, 128)
	sub, err := p.cli.SubscribeFilterLogs(ctx, q, logsCh)
	if err != nil {
		log.Printf("uniswap: subscribe error: %v", err)
		return
	}
	defer sub.Unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-sub.Err():
			if err != nil {
				log.Printf("uniswap: subscription error: %v", err)
			}
			return
		case lg := <-logsCh:
			// Decode minimal fields: data contains (amount0, amount1, sqrtPriceX96, liquidity, tick)
			if len(lg.Data) < 32*5 { // rough guard
				continue
			}
			sqrtPriceX96 := new(big.Int).SetBytes(lg.Data[32*2 : 32*3])
			decs, ok := p.cfg.SymbolToDecimals[symbol]
			if !ok {
				continue
			}
			price := priceFromSqrtX96(sqrtPriceX96, decs.Dec0, decs.Dec1)

			if tickCh != nil {
				out := models.Ticker{Type: "ticker", ProductID: symbol, Price: floatToString(price)}
				select {
				case tickCh <- out:
				default:
				}
			}
			if agg != nil {
				// types.Log does not include block time; use local time as an approximation
				agg.onPrice(price, time.Now().UTC())
			}
		}
	}
}

// candleAgg aggregates spot updates into OHLCV buckets.
type candleAgg struct {
	out      chan<- models.Candle
	interval time.Duration
	symbol   string
	dec0     int
	dec1     int

	mu    sync.Mutex
	start time.Time
	open  float64
	high  float64
	low   float64
	close float64
}

func (c *candleAgg) onPrice(price float64, t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	bucketStart := t.Truncate(c.interval)
	if c.start.IsZero() || bucketStart.After(c.start) {
		if !c.start.IsZero() {
			c.out <- models.Candle{
				Start:     c.start.UTC().Format(time.RFC3339),
				High:      c.high,
				Low:       c.low,
				Open:      c.open,
				Close:     c.close,
				Volume:    0,
				ProductID: c.symbol,
			}
		}
		c.start = bucketStart
		c.open = price
		c.high = price
		c.low = price
		c.close = price
		return
	}

	if price > c.high {
		c.high = price
	}
	if price < c.low {
		c.low = price
	}
	c.close = price
}

// priceFromSqrtX96 computes token1 per token0 price adjusted by decimals.
func priceFromSqrtX96(sqrtX96 *big.Int, dec0, dec1 int) float64 {
	if sqrtX96.Sign() == 0 {
		return 0
	}
	// P = (sqrtX96^2 / 2^192) * 10^(dec0 - dec1)
	num := new(big.Float).SetInt(new(big.Int).Mul(sqrtX96, sqrtX96))
	den := new(big.Float).SetInt(new(big.Int).Lsh(big.NewInt(1), 192))
	q := new(big.Float).Quo(num, den)
	// decimals adj
	pow := new(big.Float).SetFloat64(math.Pow10(dec0 - dec1))
	q.Mul(q, pow)
	val, _ := q.Float64()
	return val
}

func floatToString(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

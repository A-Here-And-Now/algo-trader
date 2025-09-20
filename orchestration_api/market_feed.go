package main

import (
	"context"
	"math/rand"
	"time"
)

// MarketDataFeed defines how to start per-token streams
type MarketDataFeed interface {
	StartPriceStream(ctx context.Context, symbol string, out chan<- PriceTick) func()
	StartCandleStream(ctx context.Context, symbol string, interval time.Duration, out chan<- Candle) func()
}

// defaultFeed is used by Manager. Replace with a real Coinbase feed implementation.
var defaultFeed MarketDataFeed

// SimulatedFeed emits synthetic ticks and candles for development
type SimulatedFeed struct{}

func (s *SimulatedFeed) StartPriceStream(ctx context.Context, symbol string, out chan<- PriceTick) func() {
	child, cancel := context.WithCancel(ctx)
	go func() {
		t := time.NewTicker(1 * time.Second)
		defer t.Stop()
		price := 100.0 + rand.Float64()*10
		for {
			select {
			case <-child.Done():
				return
			case ts := <-t.C:
				// random walk
				price += (rand.Float64() - 0.5) * 0.5
				select {
				case out <- PriceTick{Symbol: symbol, Price: price, Ts: ts}:
				default:
				}
			}
		}
	}()
	return cancel
}

func (s *SimulatedFeed) StartCandleStream(ctx context.Context, symbol string, interval time.Duration, out chan<- Candle) func() {
	child, cancel := context.WithCancel(ctx)
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		open := 100.0 + rand.Float64()*10
		for {
			start := time.Now()
			high := open
			low := open
			close := open
			vol := 0.0
			// build the candle over the interval using smaller steps
			step := time.NewTicker(interval / 10)
			for i := 0; i < 10; i++ {
				select {
				case <-child.Done():
					step.Stop()
					return
				case <-step.C:
					move := (rand.Float64() - 0.5) * 1.0
					close = close + move
					if close > high {
						high = close
					}
					if close < low {
						low = close
					}
					vol += rand.Float64() * 10
				}
			}
			step.Stop()
			c := Candle{Symbol: symbol, Open: open, High: high, Low: low, Close: close, Volume: vol, StartTs: start, EndTs: time.Now()}
			open = close
			select {
			case <-child.Done():
				return
			case <-t.C:
				select {
				case out <- c:
				default:
				}
			}
		}
	}()
	return cancel
}

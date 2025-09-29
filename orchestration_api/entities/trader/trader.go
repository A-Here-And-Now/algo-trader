// trader.go
package trader

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/coinbase"
)

type Trader struct {
	cfg                         TradeCfg
	ctx                         context.Context
	cancel                      context.CancelFunc
	updates                     chan TradeCfg
	signalCh                    chan models.Signal
	actualPositionToken         float64
	actualPositionUSD           float64 // actual position in USD
	usdAmountPerFulfilledOrders float64 // actual position in USD without gains or losses
	targetPositionUSD           float64 // target position in USD
	orderFeed                   chan coinbase.OrderUpdate
	pendingOrder                *models.PendingOrder
	currentPriceUSDPerToken     float64
	priceFeed                   chan models.Ticker
	CoinbaseClient              *coinbase.CoinbaseClient
}

func (t *Trader) getTargetPositionPct() float64 {
	return t.targetPositionUSD / t.cfg.AllocatedFunds
}

func (t *Trader) getActualPositionPct() float64 {
	return t.actualPositionUSD / t.cfg.AllocatedFunds
}

// NewTrader builds a trader instance from a config.
func NewTrader(cfg TradeCfg, ctx context.Context, cancel context.CancelFunc, updates chan TradeCfg, signalCh chan models.Signal, orderFeed chan coinbase.OrderUpdate, startingTokenBalance float64) *Trader {
	return &Trader{cfg: cfg, ctx: ctx, cancel: cancel, updates: updates, signalCh: signalCh, orderFeed: orderFeed, actualPositionToken: startingTokenBalance}
}

// Run is the long‑running loop that talks to an exchange, processes signals, etc.
// It stops when ctx is cancelled.
func (t *Trader) Run() {
	log.Printf("[Trader %s] started – AllocatedFunds=%v", t.cfg.Symbol, t.cfg.AllocatedFunds)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case price := <-t.priceFeed:
			t.handlePriceUpdate(price)
		case update := <-t.updates:
			t.adjustTargetPositionAccordingToAllocatedFundsUpdate(update)
		case sig := <-t.signalCh:
			t.handleSignal(sig)
		case ord := <-t.orderFeed:
			t.handleOrderUpdate(ord)
		case <-ticker.C:
			t.executeTradesToMakeActualTrackTarget()
		case <-t.ctx.Done():
			log.Printf("[Trader %s] Context done...  closing positions", t.cfg.Symbol)
			t.cancelPendingOrderWithTimeout()
			t.sellTokensWithTimeout()		
			return
		}
	}
}

func (t *Trader) handlePriceUpdate(price models.Ticker) {
	t.currentPriceUSDPerToken, _ = strconv.ParseFloat(price.Price, 64)
	t.actualPositionUSD = t.actualPositionToken * t.currentPriceUSDPerToken
	if (t.usdAmountPerFulfilledOrders == 0) { // with this, the current logic can know about the pre-existing position and adjust accordingly
		t.usdAmountPerFulfilledOrders = t.actualPositionUSD
	}
}

func (t *Trader) adjustTargetPositionAccordingToAllocatedFundsUpdate(update TradeCfg) {
	// adjust target position to maintain same target percentage after allocation change
	oldTargetPct := t.getTargetPositionPct()
	log.Printf("[Trader %s] AllocatedFunds updating from %v to %v", t.cfg.Symbol, t.cfg.AllocatedFunds, update.AllocatedFunds)
	if (t.cfg.Strategy != update.Strategy) {
		t.targetPositionUSD = 0
	}
	t.cfg = update
	newTargetPct := t.getTargetPositionPct()
	if (newTargetPct != oldTargetPct) {
		targetPositionIncrease := (newTargetPct - oldTargetPct) * t.cfg.AllocatedFunds / 100.0
		t.targetPositionUSD += targetPositionIncrease
		log.Printf("[Trader %s] Target position increased by %v to a resulting value of %v", t.cfg.Symbol, targetPositionIncrease, t.targetPositionUSD)
	}
}

// handleSignal executes buy/sell respecting rules on allocated funds and bounds 0..100
func (t *Trader) handleSignal(s models.Signal) {
	log.Printf("[Trader %s] Signal received: Percent=%v Type=%s", t.cfg.Symbol, s.Percent, s.Type)
	if s.Percent <= 0 {
		return
	}
	pct := s.Percent
	switch s.Type {
	case enum.SignalBuy:
		// Buy percent pertains to allocated funds but cannot exceed 100% target
		t.targetPositionUSD += pct * t.cfg.AllocatedFunds / 100.0
		if t.targetPositionUSD > t.cfg.AllocatedFunds {
			t.targetPositionUSD = t.cfg.AllocatedFunds
		}
	case enum.SignalSell:
		// Sell percent pertains to position if position > 100, else allocated funds percent
		targetPct := t.getTargetPositionPct()
		actualPct := t.getActualPositionPct()
		if actualPct > targetPct {
			pct *= actualPct / targetPct
		}
		if pct > targetPct {
			pct = targetPct
		}
		t.targetPositionUSD -= pct * t.cfg.AllocatedFunds / 100.0
	default:
		// hold not emitted
	}
}

func (t *Trader) handleOrderUpdate(up coinbase.OrderUpdate) {
	if t.pendingOrder != nil {
		if t.pendingOrder.OrderID == up.OrderID && up.Status == "FILLED" {
			leaves, _ := strconv.ParseFloat(up.Leaves, 64)
			if leaves != t.pendingOrder.CurrentAmountLeftToBeFilledInUSD {
				cumulativeQuantity, _ := strconv.ParseFloat(up.FilledQty, 64)
				filledUSD := t.pendingOrder.OriginalAmountInUSD - leaves
				filledTokens := cumulativeQuantity - t.pendingOrder.AlreadyFilledInTokens
				alreadyFilledUSD := t.pendingOrder.AlreadyFilledInUSD
				alreadyFilledTokens := t.pendingOrder.AlreadyFilledInTokens

				t.updatePendingOrderBalances(leaves, filledUSD, filledTokens)

				// now we update the actual position (the one w/o gains or losses) minus the amount that we already delta'd our actual position from previous order updates
				if up.Side == "BUY" {
					t.usdAmountPerFulfilledOrders += filledUSD - alreadyFilledUSD
					t.actualPositionToken += filledTokens - alreadyFilledTokens
				} else {
					t.usdAmountPerFulfilledOrders -= filledUSD - alreadyFilledUSD
					t.actualPositionToken -= filledTokens - alreadyFilledTokens
				}
			}
		}
	}
}

func (t *Trader) executeTradesToMakeActualTrackTarget() {
	if t.hasPendingOrder() {
		return
	}
	var tolerance float64 = t.cfg.AllocatedFunds * 0.01
	var deficitOrExcess float64 = t.getTotalPositionAsFulfilledOrdersPlusPending() - t.targetPositionUSD
	log.Printf("[Trader %s] deficitOrExcess: %v, tolerance: %v", t.cfg.Symbol, deficitOrExcess, tolerance)
	if deficitOrExcess > 0 && deficitOrExcess > tolerance {
		t.submitBuyToCoinbase(deficitOrExcess)
	} else if deficitOrExcess < 0 && deficitOrExcess < -tolerance {
		t.submitSellToCoinbase(-deficitOrExcess)
	}
}

// executeWithTimeout is a helper function for executing operations with timeout and consistent error handling
func (t *Trader) executeWithTimeout(timeoutSeconds int, operationName string, operation func(context.Context) error) error {
    // Create context with timeout
    ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
    defer cancel()

    err := operation(ctx)
    if err != nil {
        if ctx.Err() == context.DeadlineExceeded {
            log.Printf("%s timed out for %s", operationName, t.cfg.Symbol)
        } else {
            log.Printf("%s failed for %s: %v", operationName, t.cfg.Symbol, err)
        }
        return err
    }

    log.Printf("%s succeeded for %s", operationName, t.cfg.Symbol)
    return nil
}

func (t *Trader) cancelPendingOrderWithTimeout() error {
    if t.pendingOrder == nil {
        return nil
    }

    orderID := t.pendingOrder.OrderID
    err := t.executeWithTimeout(10, "Cancel order", func(ctx context.Context) error {
        return t.CoinbaseClient.CancelOrders(ctx, orderID)
    })

    if err == nil {
        log.Printf("Successfully cancelled order %s", orderID)
        t.clearPendingOrder()
    }

    return err
}

func (t *Trader) sellTokensWithTimeout() error {
    if t.actualPositionToken <= 0 {
        return nil
    }

    symbol := t.cfg.Symbol
    amount := t.actualPositionToken
    err := t.executeWithTimeout(10, "Sell tokens", func(ctx context.Context) error {
        _, err := t.CoinbaseClient.SellTokens(symbol, amount)
        return err
    })

    if err == nil {
        log.Printf("Successfully submitted sell order for tokens of %s", symbol)
    }

    return err
}

func (t *Trader) getTotalPositionAsFulfilledOrdersPlusPending() float64 {
	total := t.usdAmountPerFulfilledOrders
	if t.hasPendingOrder() {
		switch t.pendingOrder.OrderType {
		case enum.SignalBuy:
			total += t.pendingOrder.CurrentAmountLeftToBeFilledInUSD
		case enum.SignalSell:
			total -= t.pendingOrder.CurrentAmountLeftToBeFilledInUSD
		}
	}
	return total
}

func (t *Trader) submitBuyToCoinbase(amount float64) error {
	response, err := t.CoinbaseClient.CreateOrder(t.ctx, t.cfg.Symbol, amount, true)
	if err != nil {
		log.Printf("failed to submit buy to coinbase: %v", err)
		return err
	}
	log.Printf("submitted buy to coinbase: %v", response)
	t.setPendingOrder(t.getPendingOrderFromResponse(response, enum.SignalBuy, amount))
	return nil
}

func (t *Trader) submitSellToCoinbase(amount float64) error {
	response, err := t.CoinbaseClient.CreateOrder(t.ctx, t.cfg.Symbol, amount, true)
	if err != nil {
		log.Printf("failed to submit buy to coinbase: %v", err)
		return err
	}
	log.Printf("submitted buy to coinbase: %v", response)
	t.setPendingOrder(t.getPendingOrderFromResponse(response, enum.SignalBuy, amount))
	return nil
}

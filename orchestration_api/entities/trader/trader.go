// trader.go
package trader

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	exchange "github.com/A-Here-And-Now/algo-trader/orchestration_api/exchange"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
)

type Trader struct {
	cfg                         TradeCfg
	ctx                         context.Context
	cancel                      context.CancelFunc
	updates                     chan TradeCfg
	signalCh                    chan models.Signal
	state                       models.TraderState
	exchange                    exchange.IExchange
}

// NewTrader builds a trader instance from a config.
func NewTrader(cfg TradeCfg, ctx context.Context, cancel context.CancelFunc, updates chan TradeCfg, signalCh chan models.Signal, startingTokenBalance float64, exchange exchange.IExchange) *Trader {
	return &Trader{cfg: cfg, ctx: ctx, cancel: cancel, updates: updates, signalCh: signalCh, state: models.TraderState{ActualPositionToken: startingTokenBalance}, exchange: exchange}
}

func (t *Trader) Run() {
	log.Printf("[Trader %s] started – AllocatedFunds=%v", t.cfg.Symbol, t.cfg.AllocatedFunds)

	tickerCh, tickerCleanup, err := t.exchange.SubscribeToTicker(t.cfg.Symbol)
	if err != nil {
		log.Printf("[Trader %s] failed to subscribe to ticker: %v", t.cfg.Symbol, err)
		return
	}
	defer tickerCleanup()
	orderUpdateCh, orderUpdateCleanup, err := t.exchange.SubscribeToOrderUpdates(t.cfg.Symbol)
	if err != nil {
		log.Printf("[Trader %s] failed to subscribe to ticker: %v", t.cfg.Symbol, err)
		return
	}
	defer orderUpdateCleanup()
	ticker := time.NewTicker(enum.GetTimeDurationFromCandleSize(t.cfg.CandleSize) / 5)
	defer ticker.Stop()
	for {
		select {
		case price := <-tickerCh:
			t.handlePriceUpdate(price)
		case update := <-t.updates:
			t.adjustTargetPositionAccordingToAllocatedFundsUpdate(update)
		case sig := <-t.signalCh:
			t.handleSignal(sig)
		case ord := <-orderUpdateCh:
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

func (t *Trader) getTargetPositionPct() float64 {
	return t.state.TargetPositionUSD / t.cfg.AllocatedFunds
}

func (t *Trader) getActualPositionPct() float64 {
	return t.state.ActualPositionUSD / t.cfg.AllocatedFunds
}

func (t *Trader) handlePriceUpdate(ticker models.Ticker) {
	t.state.CurrentPriceUSDPerToken = ticker.Price
	t.state.ActualPositionUSD = t.state.ActualPositionToken * t.state.CurrentPriceUSDPerToken
	if t.state.UsdAmountPerFulfilledOrders == 0 { // with this, the current logic can know about the pre-existing position and adjust accordingly
		t.state.UsdAmountPerFulfilledOrders = t.state.ActualPositionUSD
	}
}

func (t *Trader) updateCfg(cfg TradeCfg) {
	t.cfg = cfg
}

func (t *Trader) adjustTargetPositionAccordingToAllocatedFundsUpdate(update TradeCfg) {
	// adjust target position to maintain same target percentage after allocation change
	oldTargetPct := t.getTargetPositionPct()
	log.Printf("[Trader %s] AllocatedFunds updating from %v to %v", t.cfg.Symbol, t.cfg.AllocatedFunds, update.AllocatedFunds)
	if t.cfg.Strategy != update.Strategy {
		t.state.TargetPositionUSD = 0
	}

	t.updateCfg(update)

	newTargetPct := t.getTargetPositionPct()
	if newTargetPct != oldTargetPct {
		targetPositionIncrease := (newTargetPct - oldTargetPct) * t.cfg.AllocatedFunds / 100.0
		t.state.TargetPositionUSD += targetPositionIncrease
		log.Printf("[Trader %s] Target position increased by %v to a resulting value of %v", t.cfg.Symbol, targetPositionIncrease, t.state.TargetPositionUSD)
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
		t.state.TargetPositionUSD += pct * t.cfg.AllocatedFunds / 100.0
		if t.state.TargetPositionUSD > t.cfg.AllocatedFunds {
			t.state.TargetPositionUSD = t.cfg.AllocatedFunds
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
		t.state.TargetPositionUSD -= pct * t.cfg.AllocatedFunds / 100.0
	default:
		// hold not emitted
	}
}

func (t *Trader) handleOrderUpdate(up models.OrderUpdate) {
	if t.state.PendingOrder != nil {
		if t.state.PendingOrder.OrderID == up.OrderID && up.Status == "FILLED" {
			leaves, _ := strconv.ParseFloat(up.Leaves, 64)
			if leaves != t.state.PendingOrder.CurrentAmountLeftToBeFilledInUSD {
				cumulativeQuantity, _ := strconv.ParseFloat(up.FilledQty, 64)
				filledUSD := t.state.PendingOrder.OriginalAmountInUSD - leaves
				filledTokens := cumulativeQuantity - t.state.PendingOrder.AlreadyFilledInTokens
				alreadyFilledUSD := t.state.PendingOrder.AlreadyFilledInUSD
				alreadyFilledTokens := t.state.PendingOrder.AlreadyFilledInTokens

				t.updatePendingOrderBalances(leaves, filledUSD, filledTokens)

				// now we update the actual position (the one w/o gains or losses) minus the amount that we already delta'd our actual position from previous order updates
				if up.Side == "BUY" {
					t.state.UsdAmountPerFulfilledOrders += filledUSD - alreadyFilledUSD
					t.state.ActualPositionToken += filledTokens - alreadyFilledTokens
				} else {
					t.state.UsdAmountPerFulfilledOrders -= filledUSD - alreadyFilledUSD
					t.state.ActualPositionToken -= filledTokens - alreadyFilledTokens
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
	var deficitOrExcess float64 = t.getTotalPositionAsFulfilledOrdersPlusPending() - t.state.TargetPositionUSD
	log.Printf("[Trader %s] deficitOrExcess: %v, tolerance: %v", t.cfg.Symbol, deficitOrExcess, tolerance)
	if deficitOrExcess > 0 && deficitOrExcess > tolerance {
		t.submitBuyToCoinbase(deficitOrExcess)
	} else if deficitOrExcess < 0 && deficitOrExcess < -tolerance {
		t.submitSellToCoinbase(-deficitOrExcess)
	}
}

func (t *Trader) executeWithTimeout(timeoutSeconds int, operationName string, operation func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(t.ctx, time.Duration(timeoutSeconds)*time.Second)
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
	if t.state.PendingOrder == nil {
		return nil
	}

	orderID := t.state.PendingOrder.OrderID
	err := t.executeWithTimeout(10, "Cancel order", func(ctx context.Context) error {
		return t.exchange.CancelOrders(ctx, orderID)
	})

	if err == nil {
		log.Printf("Successfully cancelled order %s", orderID)
		t.clearPendingOrder()
	}

	return err
}

func (t *Trader) sellTokensWithTimeout() error {
	if t.state.ActualPositionToken <= 0 {
		return nil
	}

	symbol := t.cfg.Symbol
	amount := t.state.ActualPositionToken
	err := t.executeWithTimeout(10, "Sell tokens", func(ctx context.Context) error {
		_, err := t.exchange.SellTokens(ctx, symbol, amount)
		return err
	})

	if err == nil {
		log.Printf("Successfully submitted sell order for tokens of %s", symbol)
	}

	return err
}

func (t *Trader) getTotalPositionAsFulfilledOrdersPlusPending() float64 {
	total := t.state.UsdAmountPerFulfilledOrders
	if t.hasPendingOrder() {
		switch t.state.PendingOrder.OrderType {
		case enum.SignalBuy:
			total += t.state.PendingOrder.CurrentAmountLeftToBeFilledInUSD
		case enum.SignalSell:
			total -= t.state.PendingOrder.CurrentAmountLeftToBeFilledInUSD
		}
	}
	return total
}

func (t *Trader) submitBuyToCoinbase(amount float64) error {
	response, err := t.exchange.CreateOrder(t.ctx, t.cfg.Symbol, amount, true)
	if err != nil {
		log.Printf("failed to submit buy to coinbase: %v", err)
		return err
	}
	log.Printf("submitted buy to coinbase: %v", response)
	t.setPendingOrder(t.getPendingOrderFromResponse(response, enum.SignalBuy, amount))
	return nil
}

func (t *Trader) submitSellToCoinbase(amount float64) error {
	response, err := t.exchange.CreateOrder(t.ctx, t.cfg.Symbol, amount, false)
	if err != nil {
		log.Printf("failed to submit buy to coinbase: %v", err)
		return err
	}
	log.Printf("submitted buy to coinbase: %v", response)
	t.setPendingOrder(t.getPendingOrderFromResponse(response, enum.SignalBuy, amount))
	return nil
}

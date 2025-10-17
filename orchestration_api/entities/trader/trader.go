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
	cfg      TradeCfg
	ctx      context.Context
	cancel   context.CancelFunc
	updates  chan TradeCfg
	signalCh chan models.Signal
	state    models.TraderState
	exchange exchange.IExchange
	profitLossTotalChannel chan models.TokenProfitLossUpdate
	timeOfLastProfitLossReport time.Time
}

// NewTrader builds a trader instance from a config.
func NewTrader(cfg TradeCfg, ctx context.Context, cancel context.CancelFunc, updates chan TradeCfg, signalCh chan models.Signal, profitLossTotalChannel chan models.TokenProfitLossUpdate, startingTokenBalance float64, exchange exchange.IExchange) *Trader {
	return &Trader{cfg: cfg, ctx: ctx, cancel: cancel, updates: updates, signalCh: signalCh, state: models.TraderState{ActualPositionToken: startingTokenBalance}, exchange: exchange, profitLossTotalChannel: profitLossTotalChannel}
}

func (t *Trader) Run() {
	log.Printf("[Trader %s] started – AllocatedFunds=%v", t.cfg.Symbol, t.cfg.AllocatedFunds)

	tickerCh, tickerCleanup := t.exchange.SubscribeToTicker(t.cfg.Symbol)
	defer tickerCleanup()
	orderUpdateCh, orderUpdateCleanup := t.exchange.SubscribeToOrderUpdates(t.cfg.Symbol)
	defer orderUpdateCleanup()
	ticker := time.NewTicker(enum.GetTimeDurationFromCandleSize(t.cfg.CandleSize) / 5)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			log.Printf("[Trader %s] Context done... closing positions", t.cfg.Symbol)
			t.cancelPendingOrderWithTimeout()
			t.sellTokensWithTimeout()
			return

		case price, ok := <-tickerCh:
			if !ok || t.ctx.Err() != nil {
				log.Printf("[Trader %s] Ticker channel closed", t.cfg.Symbol)
				return
			}
			t.handlePriceUpdate(price)

		case ord, ok := <-orderUpdateCh:
			if !ok || t.ctx.Err() != nil {
				log.Printf("[Trader %s] Order update channel closed", t.cfg.Symbol)
				return
			}
			t.handleOrderUpdate(ord)

		case <-ticker.C:
			if t.ctx.Err() != nil {
				return
			}
			t.executeTradesToMakeActualTrackTarget()

		case update, ok := <-t.updates:
			if !ok || t.ctx.Err() != nil {
				log.Printf("[Trader %s] Updates channel closed, exiting", t.cfg.Symbol)
				return // Manager stopped us
			}
			t.adjustTargetPositionAccordingToAllocatedFundsUpdate(update)

		case sig, ok := <-t.signalCh:
			if !ok || t.ctx.Err() != nil {
				log.Printf("[Trader %s] Signal channel closed, exiting", t.cfg.Symbol)
				return // Manager stopped us
			}
			t.handleSignal(sig)
		}
	}
}

func (t *Trader) reportProfitLossTotal() {
	profitLoss := t.state.ActualPositionUSD - t.cfg.AllocatedFunds
	t.profitLossTotalChannel <- models.TokenProfitLossUpdate{Symbol: t.cfg.Symbol, ProfitLoss: profitLoss}
	log.Printf("[Trader %s] Reported profit loss total: %v", t.cfg.Symbol, profitLoss)
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
	if time.Since(t.timeOfLastProfitLossReport) > 20 * time.Second {
		t.reportProfitLossTotal()
		t.timeOfLastProfitLossReport = time.Now()
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
				t.reportProfitLossTotal()
			}
		}
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

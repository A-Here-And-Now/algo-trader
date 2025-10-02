package strategies

import (
	"log"
	"time"

	helper "github.com/A-Here-And-Now/algo-trader/orchestration_api/entities/signaler/strategy_helper"
	enum "github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	models "github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
	talib "github.com/markcheno/go-talib"
)

type MeanReversionStrategy struct {
	*helper.PositionHolder // embed – gives us .state + ConfirmSignalDelivered
	TpATRMultiplier        float64
	SlATRMultiplier        float64
}

func (s *MeanReversionStrategy) CalculateSignal(symbol string, priceStore helper.IPriceActionStore) models.Signal {
	const (
		rsiLength = 14
		// rsiOverbought := float64(80)
		rsiOversold    = float64(20)
		atrLength      = 14
		emaLengthLower = 20
		emaLengthUpper = 100
	)
	fullHistory := priceStore.GetFullMergedCandleHistory(symbol)
	closes := fullHistory.GetCloses()
	highs := fullHistory.GetHighs()
	lows := fullHistory.GetLows()
	opens := fullHistory.GetOpens()

	if len(closes) < 100 {
		return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
	}

	idx := len(closes) - 1
	lastClose := closes[idx]

	// indicators
	rsi := talib.Rsi(closes, rsiLength)
	atr := talib.Atr(highs, lows, closes, atrLength)
	emaLower := talib.Ema(closes, emaLengthLower)
	emaUpper := talib.Ema(closes, emaLengthUpper)

	// entry conditions
	rsiLongOK := rsi[idx] < rsiOversold
	// rsiShortOK := rsi[idx] > rsiOverbought
	// validBearishFVG := highs[idx-12] < lows[idx]
	validBullishFVG := lows[idx-12] > highs[idx]
	bullishSignal := validBullishFVG && lastClose > opens[idx] && rsiLongOK
	// bearishSignal := validBearishFVG && lastClose < opens[idx] && rsiShortOK

	ps := s.State[symbol]

	// If in a position, only allow the opposing signal when TP/SL is hit
	if ps.InPosition {
		// if ps.side == enum.SignalBuy { // currently long, only SELL can trigger
		if lastClose >= ps.TakeProfit || lastClose <= ps.StopLoss {
			log.Println("MeanReversionStrategy:", symbol, "Long exit (TP/SL)")
			ps = &helper.PositionState{}
			s.State[symbol] = ps
			return models.Signal{Symbol: symbol, Type: enum.SignalSell, Percent: 100, Time: time.Now()}
		}
		return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
		// }
		// // currently short, only BUY can trigger
		// if lastClose <= ps.takeProfit || lastClose >= ps.stopLoss {
		// 	log.Println("MeanReversionStrategy:", symbol, "Short exit (TP/SL)")
		// 	ps = positionState{}
		// 	s.state[symbol] = ps
		// 	return models.Signal{Symbol: symbol, Type: enum.SignalBuy, Percent: 100, Time: time.Now()}
		// }
		// return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
	}

	// Not in a position: consider entries
	if bullishSignal && lastClose < emaLower[idx] && lastClose < emaUpper[idx] {
		ps.Side = enum.SignalBuy
		ps.EntryPrice = lastClose
		ps.TakeProfit = lastClose + atr[idx]*s.TpATRMultiplier
		ps.StopLoss = lastClose - atr[idx]*s.SlATRMultiplier
		s.State[symbol] = ps
		log.Println("MeanReversionStrategy:", symbol, "Long entry")
		return models.Signal{Symbol: symbol, Type: enum.SignalBuy, Percent: 100, Time: time.Now()}
	}

	// if bearishSignal && lastClose > emaUpper[idx] && lastClose > emaLower[idx] {
	// 	ps.inPosition = true
	// 	ps.side = enum.SignalSell
	// 	ps.entryPrice = lastClose
	// 	ps.takeProfit = lastClose - atr[idx]*s.tpATRMultiplier
	// 	ps.stopLoss = lastClose + atr[idx]*s.slATRMultiplier
	// 	s.state[symbol] = ps
	// 	log.Println("MeanReversionStrategy:", symbol, "Short entry")
	// 	return models.Signal{Symbol: symbol, Type: enum.SignalSell, Percent: 100, Time: time.Now()}
	// }

	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

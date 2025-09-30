package signaler

import (
	"log"
	"time"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
	talib "github.com/markcheno/go-talib"
)

type Strategy interface {
	ConfirmSignalDelivered(symbol string, signalType enum.SignalType)
	CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal
}

func NewStrategy(strategy enum.Strategy) Strategy {
	switch strategy {
	case enum.MeanReversion:
		return &MeanReversionStrategy{
			state:           make(map[string]*positionState),
			tpATRMultiplier: 4.0,
			slATRMultiplier: 2.0,
		}
	case enum.TrendFollowingWithMomentumConfirmation:
		return &TrendFollowingWithMomentumConfirmationStrategy{
			state:           make(map[string]*positionState),
		}
	case enum.CandlestickSignalAggregation:
		return &CandlestickSignalAggregationStrategy{
			state:           make(map[string]*positionState),
		}
	case enum.RenkoCandlesticks:
		return &RenkoCandlesticksStrategy{
			state:           make(map[string]*positionState),
		}
	case enum.HeikenAshi:
		return &HeikenAshiStrategy{
			state:           make(map[string]*positionState),
		}
	case enum.DonchianChannel:
		return &DonchianChannelStrategy{
			state:           make(map[string]*positionState),
		}
	case enum.TrendlineBreakout:
		return &TrendlineBreakoutStrategy{
			state:           make(map[string]*positionState),
		}
	case enum.Supertrend:
		return &SupertrendStrategy{
			state:           make(map[string]*positionState),
		}
	case enum.GroverLlorensActivator:
		return &GroverLlorensActivatorStrategy{
			state:           make(map[string]*positionState),
		}
	}
	return nil
}

type positionState struct {
	inPosition bool
	side       enum.SignalType // SignalBuy when long, SignalSell when short
	entryPrice float64
	takeProfit float64
	stopLoss   float64
}
/*******************************************/
/***************MeanReversion***************/
/*******************************************/

type MeanReversionStrategy struct {
	state           map[string]*positionState
	tpATRMultiplier float64
	slATRMultiplier float64
}

func (s *MeanReversionStrategy) ConfirmSignalDelivered(symbol string, signalType enum.SignalType) {
	s.state[symbol].inPosition = signalType == enum.SignalBuy;
}

func (s *MeanReversionStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
	rsiLength := 14
	// rsiOverbought := float64(80)
	rsiOversold := float64(20)
	atrLength := 14
	emaLengthLower := 20
	emaLengthUpper := 100
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

	ps := s.state[symbol]

	// If in a position, only allow the opposing signal when TP/SL is hit
	if ps.inPosition {
		// if ps.side == enum.SignalBuy { // currently long, only SELL can trigger
		if lastClose >= ps.takeProfit || lastClose <= ps.stopLoss {
			log.Println("MeanReversionStrategy:", symbol, "Long exit (TP/SL)")
			ps = &positionState{}
			s.state[symbol] = ps
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
		ps.side = enum.SignalBuy
		ps.entryPrice = lastClose
		ps.takeProfit = lastClose + atr[idx]*s.tpATRMultiplier
		ps.stopLoss = lastClose - atr[idx]*s.slATRMultiplier
		s.state[symbol] = ps
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

/*******************************************/
/***TrendFollowingWithMomentumConfirmation**/
/*******************************************/
type TrendFollowingWithMomentumConfirmationStrategy struct{
	state           map[string]*positionState
}

func (s *TrendFollowingWithMomentumConfirmationStrategy) ConfirmSignalDelivered(symbol string, signalType enum.SignalType) {
	s.state[symbol].inPosition = signalType == enum.SignalBuy;
}

func (s *TrendFollowingWithMomentumConfirmationStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
	

	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

/*******************************************/
/*********CandlestickSignalAggregation******/
/*******************************************/
type CandlestickSignalAggregationStrategy struct{
	state           map[string]*positionState
}

func (s *CandlestickSignalAggregationStrategy) ConfirmSignalDelivered(symbol string, signalType enum.SignalType) {
	s.state[symbol].inPosition = signalType == enum.SignalBuy;
}

func (s *CandlestickSignalAggregationStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

/*******************************************/
/***********RenkoCandlesticks***************/
/*******************************************/
type RenkoCandlesticksStrategy struct{
	state           map[string]*positionState
}

func (s *RenkoCandlesticksStrategy) ConfirmSignalDelivered(symbol string, signalType enum.SignalType) {
	s.state[symbol].inPosition = signalType == enum.SignalBuy;
}

func (s *RenkoCandlesticksStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

/*******************************************/
/***************HeikenAshi******************/
/*******************************************/
type HeikenAshiStrategy struct{
	state           map[string]*positionState
}

func (s *HeikenAshiStrategy) ConfirmSignalDelivered(symbol string, signalType enum.SignalType) {
	s.state[symbol].inPosition = signalType == enum.SignalBuy;
}

func (s *HeikenAshiStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

/*******************************************/
/***************DonchianChannel*************/
/*******************************************/
type DonchianChannelStrategy struct{
	state           map[string]*positionState
}

func (s *DonchianChannelStrategy) ConfirmSignalDelivered(symbol string, signalType enum.SignalType) {
	s.state[symbol].inPosition = signalType == enum.SignalBuy;
}

func (s *DonchianChannelStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

/*******************************************/
/***************TrendlineBreakout***********/
/*******************************************/
type TrendlineBreakoutStrategy struct{
	state           map[string]*positionState
}

func (s *TrendlineBreakoutStrategy) ConfirmSignalDelivered(symbol string, signalType enum.SignalType) {
	s.state[symbol].inPosition = signalType == enum.SignalBuy;
}

func (s *TrendlineBreakoutStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

/*******************************************/
/****************Supertrend*****************/
/*******************************************/
type SupertrendStrategy struct{
	state           map[string]*positionState
}

func (s *SupertrendStrategy) ConfirmSignalDelivered(symbol string, signalType enum.SignalType) {
	s.state[symbol].inPosition = signalType == enum.SignalBuy;
}

func (s *SupertrendStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

/*******************************************/
/***********GroverLlorensActivator**********/
/*******************************************/
type GroverLlorensActivatorStrategy struct{
	state           map[string]*positionState
}

func (s *GroverLlorensActivatorStrategy) ConfirmSignalDelivered(symbol string, signalType enum.SignalType) {
	s.state[symbol].inPosition = signalType == enum.SignalBuy;
}

func (s *GroverLlorensActivatorStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

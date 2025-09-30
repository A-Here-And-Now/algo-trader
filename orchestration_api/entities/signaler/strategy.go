package signaler

import (
    "log"
    "time"

    "github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
    "github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
    talib "github.com/markcheno/go-talib"
)

/* ------------------------------------------------------------------------ PUBLIC INTERFACE ------------------------------------------------------------------------ */
type Strategy interface {
    ConfirmSignalDelivered(symbol string, signalType enum.SignalType)
    CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal
}

/* ------------------------------------------------------------------------ SHARED STATE HELPERS ------------------------------------------------------------------------ */
type positionState struct {
    inPosition bool
    side       enum.SignalType
    entryPrice float64
    takeProfit float64
    stopLoss   float64
}

// Holds the map and the common ConfirmSignalDelivered implementation.
type positionHolder struct {
    state map[string]*positionState
}

func newPositionHolder() *positionHolder {
    return &positionHolder{state: make(map[string]*positionState)}
}

func (h *positionHolder) ConfirmSignalDelivered(symbol string, signalType enum.SignalType) {
    if _, ok := h.state[symbol]; !ok {
        h.state[symbol] = &positionState{}
    }
    h.state[symbol].inPosition = signalType == enum.SignalBuy
}

/* ------------------------------------------------------------------------ FACTORY ------------------------------------------------------------------------ */
func NewStrategy(strategy enum.Strategy) Strategy {
    switch strategy {
    case enum.MeanReversion:
        return &MeanReversionStrategy{
            positionHolder:   newPositionHolder(),
            tpATRMultiplier: 4.0,
            slATRMultiplier: 2.0,
        }
    case enum.TrendFollowingWithMomentumConfirmation:
        return &TrendFollowingWithMomentumConfirmationStrategy{
            positionHolder: newPositionHolder(),
        }
    case enum.CandlestickSignalAggregation:
        return &CandlestickSignalAggregationStrategy{
            positionHolder: newPositionHolder(),
        }
	case enum.RenkoCandlesticks:
		return &RenkoCandlesticksStrategy{
			positionHolder: newPositionHolder(),
		}
	case enum.HeikenAshi:
		return &HeikenAshiStrategy{
			positionHolder: newPositionHolder(),
		}
	case enum.DonchianChannel:
		return &DonchianChannelStrategy{
			positionHolder: newPositionHolder(),
		}
	case enum.TrendlineBreakout:
		return &TrendlineBreakoutStrategy{
			positionHolder: newPositionHolder(),
		}
	case enum.Supertrend:
		return &SupertrendStrategy{
			positionHolder: newPositionHolder(),
		}
	case enum.GroverLlorensActivator:
		return &GroverLlorensActivatorStrategy{
			positionHolder: newPositionHolder(),
		}
    }
    return nil
}

/* ------------------------------------------------------------------------ CONCRETE STRATEGIES ------------------------------------------------------------------------ */

/*******************************************/
/***********MeanReversionStrategy***********/
/*******************************************/
type MeanReversionStrategy struct {
    *positionHolder                // embed – gives us .state + ConfirmSignalDelivered
    tpATRMultiplier float64
    slATRMultiplier float64
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
/*****TrendFollowingWithMomConfStrategy*****/
/*******************************************/
type TrendFollowingWithMomentumConfirmationStrategy struct{ *positionHolder }

func (s *TrendFollowingWithMomentumConfirmationStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
    
    return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

/*******************************************/
/****CandlestickSignalAggregationStrategy***/
/*******************************************/
type CandlestickSignalAggregationStrategy struct{ *positionHolder }

func (s *CandlestickSignalAggregationStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {

    return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

/*******************************************/
/***********RenkoCandlesticks***************/
/*******************************************/
type RenkoCandlesticksStrategy struct{ *positionHolder }

func (s *RenkoCandlesticksStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

/*******************************************/
/***************HeikenAshi******************/
/*******************************************/
type HeikenAshiStrategy struct{ *positionHolder }

func (s *HeikenAshiStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

/*******************************************/
/***************DonchianChannel*************/
/*******************************************/
type DonchianChannelStrategy struct{ *positionHolder }

func (s *DonchianChannelStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

/*******************************************/
/***************TrendlineBreakout***********/
/*******************************************/
type TrendlineBreakoutStrategy struct{ *positionHolder }

func (s *TrendlineBreakoutStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

/*******************************************/
/****************Supertrend*****************/
/*******************************************/
type SupertrendStrategy struct{	*positionHolder }

func (s *SupertrendStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

/*******************************************/
/***********GroverLlorensActivator**********/
/*******************************************/
type GroverLlorensActivatorStrategy struct{	*positionHolder }

func (s *GroverLlorensActivatorStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

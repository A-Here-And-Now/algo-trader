package signaler

import (
	"time"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
)

type Strategy interface {
	CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal
}

func NewStrategy(strategy enum.Strategy) Strategy {
	switch strategy {
	case enum.MeanReversion:
		return &MeanReversionStrategy{}
	case enum.TrendFollowingWithMomentumConfirmation:
		return &TrendFollowingWithMomentumConfirmationStrategy{}
	case enum.CandlestickSignalAggregation:
		return &CandlestickSignalAggregationStrategy{}
	case enum.RenkoCandlesticks:
		return &RenkoCandlesticksStrategy{}
	case enum.HeikenAshi:
		return &HeikenAshiStrategy{}
	case enum.DonchianChannel:
		return &DonchianChannelStrategy{}
	case enum.TrendlineBreakout:
		return &TrendlineBreakoutStrategy{}
	case enum.Supertrend:
		return &SupertrendStrategy{}
	case enum.GroverLlorensActivator:
		return &GroverLlorensActivatorStrategy{}
	}
	return nil
}

/*******************************************/
/***************MeanReversion***************/
/*******************************************/
type MeanReversionStrategy struct {}

func (s *MeanReversionStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

/*******************************************/
/***TrendFollowingWithMomentumConfirmation**/
/*******************************************/
type TrendFollowingWithMomentumConfirmationStrategy struct {}

func (s *TrendFollowingWithMomentumConfirmationStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

/*******************************************/
/*********CandlestickSignalAggregation******/
/*******************************************/
type CandlestickSignalAggregationStrategy struct {}

func (s *CandlestickSignalAggregationStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

/*******************************************/
/***********RenkoCandlesticks***************/
/*******************************************/
type RenkoCandlesticksStrategy struct {}

func (s *RenkoCandlesticksStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

/*******************************************/
/***************HeikenAshi******************/
/*******************************************/
type HeikenAshiStrategy struct {}

func (s *HeikenAshiStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

/*******************************************/
/***************DonchianChannel*************/
/*******************************************/
type DonchianChannelStrategy struct {}

func (s *DonchianChannelStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

/*******************************************/
/***************TrendlineBreakout***********/
/*******************************************/
type TrendlineBreakoutStrategy struct {}

func (s *TrendlineBreakoutStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

/*******************************************/
/****************Supertrend*****************/
/*******************************************/
type SupertrendStrategy struct {}

func (s *SupertrendStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

/*******************************************/
/***********GroverLlorensActivator**********/
/*******************************************/
type GroverLlorensActivatorStrategy struct {}

func (s *GroverLlorensActivatorStrategy) CalculateSignal(symbol string, priceStore PriceActionStore) models.Signal {
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}
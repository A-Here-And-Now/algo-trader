package signaler

import (
	strategies "github.com/A-Here-And-Now/algo-trader/orchestration_api/entities/signaler/strategies"
	helper "github.com/A-Here-And-Now/algo-trader/orchestration_api/entities/signaler/strategy_helper"
	enum "github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	models "github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
)

/* ------------------------------------------------------------------------ PUBLIC INTERFACE ------------------------------------------------------------------------ */
type Strategy interface {
	ConfirmSignalDelivered(symbol string, signal models.Signal)
	CalculateSignal(symbol string, priceStore helper.IPriceActionStore) models.Signal
	UpdateTrailingStop(symbol string, ticker models.Ticker)
}

/* ------------------------------------------------------------------------ FACTORY ------------------------------------------------------------------------ */
func NewStrategy(strategy enum.Strategy) Strategy {
	switch strategy {
	case enum.MeanReversion:
		return &strategies.MeanReversionStrategy{
			PositionHolder:  helper.NewPositionHolder(),
			TpATRMultiplier: 3.50, SlATRMultiplier: 1.75,
		}
	case enum.TrendFollowing:
		return &strategies.TrendFollowingStrategy{
			PositionHolder: helper.NewPositionHolder(),
			MaType: "SMA", ShortMALen: 9, LongMALen: 21, BbLen: 20, BbMul: 2.0, RsiLen: 14, RsiLongTh: 55.0, RsiShortTh: 45.0, MacdFastLen: 12, MacdSlowLen: 26,
			MacdSignalLen: 9, StochLen: 14, StochSmooth: 3, StochOverbought: 80.0, StochOversold: 20.0, AdxLen: 14, AdxThreshold: 25.0, TsAtrMult: 1.5, TpAtrMult: 4,
		}
	case enum.CandlestickAggregation:
		return &strategies.CandlestickAggregationStrategy{
			PositionHolder:     helper.NewPositionHolder(),
			TsAtrMult: 1.25, TpAtrMult: 3.5, AtrLen: 14, MaLen: 20, HigherTfmALen: 50, SwingPivotLength: 10, SrTolerancePerc: 0.01,
			VolumeMALen: 20, VolumeSpikeMul: 1.5, LongBodyAtrMul: 0.8, SmallBodyAtrMul: 0.3, MinAvgStrength: 7.0, MinPatternStrength: 5.0,
		}
	case enum.RenkoCandlesticks:
		return &strategies.RenkoCandlesticksStrategy{
			PositionHolder:    helper.NewPositionHolder(),
			AtrLen: 26, StopLossPct: 10.0, TakeProfitPct: 50.0, BrickSizeConstant: 1.5,
		}
	case enum.HeikenAshi:
		return &strategies.HeikenAshiStrategy{
			PositionHolder:    helper.NewPositionHolder(),
			AtrPeriod: 26, AtrLineMultiplier: 4.0, TpATRMultiplier: 3.50, SlATRMultiplier: 1.75, NumEmaPeriods: 20,
		}
	case enum.TurtleTrader:
		return &strategies.TurtleTraderStrategy{
			PositionHolder:       helper.NewPositionHolder(),
			NumberOfPeriods: 26, PredictionUnit: "atr", PredictionMultiplier: 4.0, UsePullbackFilter: true,
		}
	case enum.TrendlineBreakout:
		return &strategies.TrendlineBreakoutStrategy{
			PositionHolder: helper.NewPositionHolder(),
			PivLR: 5, UseEmaFilter: true, EmaLen: 120, AtrLen: 14, TsAtrMult: 1.5, TpAtrMult: 4,
		}
	case enum.Supertrend:
		return &strategies.SupertrendStrategy{
			PositionHolder: helper.NewPositionHolder(),
			AtrPeriod: 26, Factor: 1.5, UseVolFilt: true, VolLen: 16, TsAtrMult: 1.5, TpAtrMult: 4,
		}
	case enum.GroverLlorensActivator:
		return &strategies.GroverLlorensActivatorStrategy{
			PositionHolder: helper.NewPositionHolder(),
			Length: 26, Mult: 1.5, TsAtrMult: 1.5, TpAtrMult: 4,
		}
	}
	return nil
}

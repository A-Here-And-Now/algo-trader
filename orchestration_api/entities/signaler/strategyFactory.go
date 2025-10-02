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
}

/* ------------------------------------------------------------------------ FACTORY ------------------------------------------------------------------------ */
func NewStrategy(strategy enum.Strategy) Strategy {
	switch strategy {
	case enum.MeanReversion:
		return &strategies.MeanReversionStrategy{
			PositionHolder:  helper.NewPositionHolder(),
			TpATRMultiplier: 3.50,
			SlATRMultiplier: 1.75,
		}
	case enum.TrendFollowing:
		return &strategies.TrendFollowingStrategy{
			PositionHolder: helper.NewPositionHolder(),
		}
	case enum.CandlestickSignalAggregation:
		return &strategies.CandlestickSignalAggregationStrategy{
			PositionHolder: helper.NewPositionHolder(),
		}
	case enum.RenkoCandlesticks:
		return &strategies.RenkoCandlesticksStrategy{
			PositionHolder:    helper.NewPositionHolder(),
			AtrLen:            26,
			StopLossPct:       10.0,
			TakeProfitPct:     50.0,
			BrickSizeConstant: 1.5,
		}
	case enum.HeikenAshi:
		return &strategies.HeikenAshiStrategy{
			PositionHolder:    helper.NewPositionHolder(),
			AtrPeriod:         26,
			AtrLineMultiplier: 4.0,
			TpATRMultiplier:   3.50,
			SlATRMultiplier:   1.75,
			NumEmaPeriods:     20,
		}
	case enum.DonchianChannel:
		return &strategies.DonchianChannelStrategy{
			PositionHolder: helper.NewPositionHolder(),
		}
	case enum.TrendlineBreakout:
		return &strategies.TrendlineBreakoutStrategy{
			PositionHolder: helper.NewPositionHolder(),
		}
	case enum.Supertrend:
		return &strategies.SupertrendStrategy{
			PositionHolder: helper.NewPositionHolder(),
		}
	case enum.GroverLlorensActivator:
		return &strategies.GroverLlorensActivatorStrategy{
			PositionHolder: helper.NewPositionHolder(),
		}
	}
	return nil
}

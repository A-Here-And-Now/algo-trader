package strategies

import (
	"time"

	helper "github.com/A-Here-And-Now/algo-trader/orchestration_api/entities/signaler/strategy_helper"
	enum "github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	models "github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
)

type HeikenAshiStrategy struct{ *helper.PositionHolder }

func (s *HeikenAshiStrategy) CalculateSignal(symbol string, priceStore helper.IPriceActionStore) models.Signal {
	hist := priceStore.GetCandleHistory(symbol)
	haCandles := hist.GetHeikenAshiCandleHistory()
	closes := haCandles.GetHeikenAshiCloses()
	highs := haCandles.GetHeikenAshiHighs()
	lows := haCandles.GetHeikenAshiLows()
	opens := haCandles.GetHeikenAshiOpens()
	volumes := haCandles.GetHeikenAshiVolumes()
	starts := haCandles.GetHeikenAshiStarts()
	
	
	
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

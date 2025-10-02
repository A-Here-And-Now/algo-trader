package strategies

import (
	"time"

	helper "github.com/A-Here-And-Now/algo-trader/orchestration_api/entities/signaler/strategy_helper"
	enum "github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	models "github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
)

type SupertrendStrategy struct{ *helper.PositionHolder }

func (s *SupertrendStrategy) CalculateSignal(symbol string, priceStore helper.IPriceActionStore) models.Signal {
	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

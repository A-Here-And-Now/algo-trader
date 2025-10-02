package strategies

import (
	"time"

	helper "github.com/A-Here-And-Now/algo-trader/orchestration_api/entities/signaler/strategy_helper"
	enum "github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	models "github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
	talib "github.com/markcheno/go-talib"
)

type RenkoCandlesticksStrategy struct{ 
	*helper.PositionHolder
	AtrLen int
	StopLossPct float64
	TakeProfitPct float64
	BrickSizeConstant float64
}

func (s *RenkoCandlesticksStrategy) CalculateSignal(symbol string, priceStore helper.IPriceActionStore) models.Signal {
	stopLossPct := s.StopLossPct
	takeProfitPct := s.TakeProfitPct
	atrLen := s.AtrLen

	hist := priceStore.GetCandleHistory(symbol)
	regCloses := hist.GetCloses()
    i := len(regCloses) - 1
    atr := talib.Atr(hist.GetHighs(), hist.GetLows(), regCloses, atrLen)
	
	if !priceStore.IsRenkoCandleHistoryBuilt() {
		priceStore.BuildRenkoCandleHistory(s.BrickSizeConstant * atr[i])
	}
	renkoCandles := priceStore.GetRenkoCandleHistory(symbol)
	
	if !priceStore.IsRenkoCandleHistoryBuilt() || renkoCandles == nil || len(renkoCandles.RenkoCandles) < 2 {
		return models.Signal{
			Symbol: symbol,
			Type:   enum.SignalHold,
			Percent: 0,
			Time:   time.Now(),
		}
	}

    renkoCloses := renkoCandles.GetRenkoCloses()
    renkoOpens  := renkoCandles.GetRenkoOpens()

    curOpen  := renkoOpens[i]
    curClose := renkoCloses[i]
    prevOpen := renkoOpens[i-1]
    prevClose := renkoCloses[i-1]

    buySignal := (prevOpen > prevClose) && (curOpen < curClose)
    sellSignal := (prevOpen < prevClose) && (curOpen > curClose)

    if !s.PositionHolder.State[symbol].InPosition && buySignal {
        return models.Signal{
            Symbol:   symbol,
            Type:     enum.SignalBuy,
            Percent:  100,
            Time:     time.Now(),
			TakeProfit: regCloses[i] * (1 + takeProfitPct / 100),
			StopLoss: regCloses[i] * (1 - stopLossPct / 100),
			Price: regCloses[i],
        }
	} else if s.PositionHolder.State[symbol].InPosition {
		isReachedTakeProfit := regCloses[i] >= s.PositionHolder.State[symbol].TakeProfit
		isReachedStopLoss := regCloses[i] <= s.PositionHolder.State[symbol].StopLoss
		if isReachedTakeProfit || isReachedStopLoss {
			return models.Signal{
				Symbol:   symbol,
				Type:     enum.SignalSell,
				Percent:  100,
				Time:     time.Now(),
			}
		}
	} else if sellSignal {
		return models.Signal{
			Symbol:   symbol,
			Type:     enum.SignalSell,
			Percent:  100,
			Time:     time.Now(),
			TakeProfit: 0,
			StopLoss: 0,
			Price: regCloses[i],
		}
	}

    return models.Signal{
        Symbol:   symbol,
        Type:     enum.SignalHold,
        Percent:  0,
        Time:     time.Now(),
    }
}

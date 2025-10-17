package strategies

import (
	"time"

	helper "github.com/A-Here-And-Now/algo-trader/orchestration_api/entities/signaler/strategy_helper"
	enum "github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	models "github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
	talib "github.com/markcheno/go-talib"
	exchange "github.com/A-Here-And-Now/algo-trader/orchestration_api/exchange"
)

type RenkoCandlesticksStrategy struct{ 
	*helper.PositionHolder
	AtrLen int
	StopLossPct float64
	TakeProfitPct float64
	BrickSizeConstant float64
}

func (s *RenkoCandlesticksStrategy) CalculateSignal(symbol string, exchange exchange.IExchange) models.Signal {
	atrLen := s.AtrLen

	hist := exchange.GetCandleHistory(symbol)
	regCloses := hist.GetCloses()
    i := len(regCloses) - 1
    atr := talib.Atr(hist.GetHighs(), hist.GetLows(), regCloses, atrLen)
	brickSize := s.BrickSizeConstant * atr[i]
	if !exchange.IsRenkoCandleHistoryBuilt(symbol) {
		exchange.BuildRenkoCandleHistory(symbol, brickSize)
	}
	renkoCandles := exchange.GetRenkoCandleHistory(symbol)
	
	if !exchange.IsRenkoCandleHistoryBuilt(symbol) || len(renkoCandles.RenkoCandles) < 2 {
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
			// since this is renko, brick size makes more sense than a hard price percent delta for take profit and stop loss
			TakeProfit: regCloses[i] + (3 * brickSize), 
			StopLoss: regCloses[i] - (1.5 * brickSize),
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

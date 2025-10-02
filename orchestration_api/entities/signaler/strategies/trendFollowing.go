package strategies

import (
	"time"

	helper "github.com/A-Here-And-Now/algo-trader/orchestration_api/entities/signaler/strategy_helper"
	enum "github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	models "github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
	talib "github.com/markcheno/go-talib"
)

type TrendFollowingStrategy struct{ *helper.PositionHolder }

func (s *TrendFollowingStrategy) CalculateSignal(symbol string,
	priceStore helper.IPriceActionStore) models.Signal {

	// ---------- MA (moving‑average) ----------
	const (
		maType     = "SMA" // the script default is SMA
		shortMALen = 9
		longMALen  = 21
	)

	// ---------- Fixed indicator parameters ----------
	const (
		atrLen          = 14
		bbLen           = 20
		bbMul           = 2.0
		rsiLen          = 14
		rsiLongTh       = 55.0
		rsiShortTh      = 45.0
		macdFastLen     = 12
		macdSlowLen     = 26
		macdSignalLen   = 9
		stochLen        = 14
		stochSmooth     = 3
		stochOverbought = 80.0
		stochOversold   = 20.0
		adxLen          = 14
		adxThreshold    = 25.0
	)

	// --------------------------------------------------------------
	// 1️⃣  Pull the merged candle history (exactly what the script uses)
	// --------------------------------------------------------------
	hist := priceStore.GetFullMergedCandleHistory(symbol)

	closes := hist.GetCloses()
	highs := hist.GetHighs()
	lows := hist.GetLows()

	// Short‑hand to the last N values (more readable than repeated len‑calculations)
	i := len(closes) - 1 // current bar index

	// opens are only needed for the MA‑cross check (the script does not use them)
	// but we keep the variable for completeness
	_ = hist.GetOpens()

	// Guard against not‑enough data
	if len(closes) < 50 {
		return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
	}

	// --------------------------------------------------------------
	// 2️⃣  Build every indicator exactly like the Pine‑Script
	// --------------------------------------------------------------
	shortMA := helper.CalcMA(closes, shortMALen, maType)
	longMA := helper.CalcMA(closes, longMALen, maType)

	// ---- Bollinger Bands -------------------------------------------------
	basis := helper.Sma(closes, bbLen)
	dev := talib.StdDev(closes, bbLen, 0) // population std‑dev
	upperBand := make([]float64, len(basis))
	lowerBand := make([]float64, len(basis))
	for i := range basis {
		upperBand[i] = basis[i] + bbMul*dev[i]
		lowerBand[i] = basis[i] - bbMul*dev[i]
	}

	// ---- Volatility filter (wide enough BB) ----------------------------
	atrVals := talib.Atr(highs, lows, closes, bbLen)
	volatilityFilter := (upperBand[len(upperBand)-1] - lowerBand[len(lowerBand)-1]) >
		(atrVals[len(atrVals)-1] * bbMul)

	// ---- BB trend filter -------------------------------------------------
	bbTrendLong := closes[i] > basis[len(basis)-1]
	bbTrendShort := closes[i] < basis[len(basis)-1]

	// ---- RSI ------------------------------------------------------------
	rsiVals := talib.Rsi(closes, rsiLen)
	rsiLong := rsiVals[len(rsiVals)-1] > rsiLongTh
	rsiShort := rsiVals[len(rsiVals)-1] < rsiShortTh

	// ---- MACD -----------------------------------------------------------
	macdLine, macdSignal, _ := talib.Macd(closes, macdFastLen, macdSlowLen, macdSignalLen)
	macdLong := macdLine[len(macdLine)-1] > macdSignal[len(macdSignal)-1]
	macdShort := macdLine[len(macdLine)-1] < macdSignal[len(macdSignal)-1]

	// ---- Stochastic ------------------------------------------------------
	stochK, stochD := talib.Stoch(highs, lows, closes,
		stochLen,    // fast‑K period (14 by default)
		stochSmooth, // slow‑K period (3 by default)
		talib.SMA,   // MA type for slow‑K (SMA)
		stochSmooth, // D period (same as smoothing)
		talib.SMA)   // MA type for D (SMA)

	// we only need the *last* value of each series for the current bar
	kLast := stochK[len(stochK)-1]
	dLast := stochD[len(stochD)-1]

	// Stochastic filter – note the direction of the inequalities
	stochLong := kLast > stochOversold && kLast > dLast    // oversold + rising
	stochShort := kLast < stochOverbought && kLast < dLast // overbought + falling

	// ---- ADX -------------------------------------------------------------
	adxVals := talib.Adx(highs, lows, closes, adxLen)
	adxOk := adxVals[len(adxVals)-1] > adxThreshold

	// --------------------------------------------------------------
	// 3️⃣  Raw MA‑cross conditions (same as Pine)
	// --------------------------------------------------------------
	buyCross := helper.CrossOver(shortMA, longMA)
	sellCross := helper.CrossUnder(shortMA, longMA)

	// --------------------------------------------------------------
	// 4️⃣  Apply *all* filters (volatility, BB‑trend, RSI, MACD,
	//    Stochastic, ADX).  The Pine‑script also had a higher‑TF trend
	//    filter which was disabled by default, so we treat it as “always
	//    true”.
	// --------------------------------------------------------------
	buyFiltered := buyCross &&
		volatilityFilter && bbTrendLong && rsiLong && macdLong && stochLong && adxOk

	sellFiltered := sellCross &&
		volatilityFilter && bbTrendShort && rsiShort && macdShort && stochShort && adxOk

	// --------------------------------------------------------------
	// 5️⃣  Return the appropriate enum.Signal
	// --------------------------------------------------------------
	if buyFiltered && !s.State[symbol].InPosition {
		return models.Signal{
			Symbol:  symbol,
			Type:    enum.SignalBuy,
			Percent: 100, // you can keep the same % as the original script
			Time:    time.Now(),
		}
	} else if sellFiltered && s.State[symbol].InPosition {
		return models.Signal{
			Symbol:  symbol,
			Type:    enum.SignalSell,
			Percent: 100,
			Time:    time.Now(),
		}
	}

	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

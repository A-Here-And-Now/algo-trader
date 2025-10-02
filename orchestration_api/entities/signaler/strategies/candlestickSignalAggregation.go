package strategies

import (
	"math"
	"time"

	helper "github.com/A-Here-And-Now/algo-trader/orchestration_api/entities/signaler/strategy_helper"
	enum "github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	models "github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
	talib "github.com/markcheno/go-talib"
)

type CandlestickSignalAggregationStrategy struct{ *helper.PositionHolder }

func (s *CandlestickSignalAggregationStrategy) CalculateSignal(symbol string, priceStore helper.IPriceActionStore) models.Signal {

	const (
		// Trend‑MA filter
		maLength      = 20
		higherTF      = "D" // higher timeframe for the HTF trend filter
		higherTFMALen = 50

		// Support / Resistance
		swingPivotLength = 10
		srTolerancePerc  = 0.01

		// Volume filter
		volumeMALen    = 20
		volumeSpikeMul = 1.5

		// ATR filter for candle body size
		atrLen          = 14
		longBodyAtrMul  = 0.8
		smallBodyAtrMul = 0.3

		// Confirmation thresholds
		minAvgStrength     = 7.0
		minPatternStrength = 5.0

		// Take‑Profit / Stop‑Loss (expressed as % → we keep them as constants but they are *not* used by the aggregation strategy)
		tpPerc = 0.05 // 5 %
		slPerc = 0.02 // 2 %

		// Back‑test date range (hard‑coded, but the aggregation strategy never uses it – we keep it only for completeness)
		// startDate = timestamp("01 Jan 2020 00:00 UTC")
		// endDate   = timestamp("31 Dec 2025 23:59 UTC")
	)

	// --------------------------------------------------------------
	// 1️⃣  Pull merged candle history (the same series the Pine‑Script uses)
	// --------------------------------------------------------------
	hist := priceStore.GetFullMergedCandleHistory(symbol)

	closes := hist.GetCloses()
	highs := hist.GetHighs()
	lows := hist.GetLows()
	opens := hist.GetOpens()
	vols := hist.GetVolumes() // assume PriceActionStore can give volume; if not, pass a zero slice
	// Short‑hand to the last N values (more readable than repeated len‑calculations)
	i := len(closes) - 1 // current bar index
	i1 := i - 1          // 1 bar ago
	i2 := i - 2
	i3 := i - 3
	i4 := i - 4
	mintick := float64(closes[i] * 0.0005)
	if mintick < 0.01 {
		mintick = 0.01
	}

	if len(closes) < 100 { // enough bars for the longest look‑back (71 patterns, some need 5‑bars)
		return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
	}

	// --------------------------------------------------------------
	// 2️⃣  Indicator calculations (identical to the Pine‑Script)
	// --------------------------------------------------------------
	atrVals := talib.Atr(highs, lows, closes, atrLen)
	atr := atrVals[len(atrVals)-1]

	// Volume SMA (for the volume‑spike filter)
	volMA := talib.Sma(vols, volumeMALen)
	volMAVal := volMA[len(volMA)-1]
	// Trend MA (simple SMA – the script uses sma)
	trendMA := talib.Sma(closes, maLength)
	trendMAVal := trendMA[len(trendMA)-1]

	// Higher‑timeframe MA (we approximate it by re‑sampling the same series
	// with the requested higher TF – the simplest way is to request a “daily”
	// series from the store; for the purpose of this port we just reuse the
	// same data and treat it as if it were the higher TF.)
	// In a real implementation you would call a separate request for TF‑D.
	htfMA := talib.Sma(closes, higherTFMALen)
	htfMAVal := htfMA[len(htfMA)-1]

	// ----- Trend, volume & S/R filters -----
	isUptrend := closes[i] > trendMAVal
	isDowntrend := closes[i] < trendMAVal

	isVolumeSpike := vols[len(vols)-1] > volMAVal*volumeSpikeMul

	// --------------------------------------------------------------
	// Bullish patterns
	// --------------------------------------------------------------

	// Neutral 1. Doji (Strength 5.0) – simple doji (small body, both shadows present)
	// defined here because doji gets checked a lot in all pattern types
	isDoji := helper.IsDoji(opens[i], closes[i], highs[i], lows[i], atr, smallBodyAtrMul, 0.1)

	// 1. Hammer (Strength 8.0)
	isHammer := helper.IsBearish(opens[i], closes[i]) &&
		helper.IsSmallBody(opens[i], closes[i],
			highs[i], lows[i],
			atr, smallBodyAtrMul, 0.2) &&
		helper.LowerShadow(opens[i], closes[i], lows[i]) > 2*helper.BodySize(opens[i], closes[i]) &&
		helper.UpperShadow(opens[i], closes[i], highs[i]) < 0.1*helper.CandleRange(highs[i], lows[i])

	// 2. Bullish Engulfing (Strength 10.0)
	isBullishEngulf := helper.IsBearish(opens[i1], closes[i1]) &&
		helper.IsBullish(opens[i], closes[i]) &&
		helper.IsEngulfing(opens[i], closes[i], opens[i1], closes[i1])

	// 3. Piercing Line (Strength 8.0)
	isPiercingLine := helper.IsBearish(opens[i1], closes[i1]) &&
		helper.IsLongBody(opens[i1], closes[i1], highs[i1], lows[i1], atr, longBodyAtrMul, 0.6) &&
		helper.IsBullish(opens[i], closes[i]) &&
		opens[i] < lows[i1] && // open gaps below prior low
		closes[i] > (opens[i1]+closes[i1])/2 && // closes above 50 % of prior body
		closes[i] < opens[i1] // still below prior open

	// 4. Morning Star (Strength 7.0)
	isMorningStar :=
		helper.IsBearish(opens[i2], closes[i2]) &&
			helper.IsLongBody(opens[i2], closes[i2], highs[i2], lows[i2], atr, longBodyAtrMul, 0.6) &&
			helper.IsSmallBody(opens[i1], closes[i1], highs[i1], lows[i1], atr, smallBodyAtrMul, 0.2) &&
			opens[i1] < closes[i2] && // the tiny middle candle opens below prior close
			helper.IsBullish(opens[i], closes[i]) &&
			helper.IsLongBody(opens[i], closes[i], highs[i], lows[i], atr, longBodyAtrMul, 0.6) &&
			closes[i] > (opens[i2]+closes[i2])/2 // closes above midpoint of first candle

	// 5. Three White Soldiers (Strength 9.0)
	is3WhiteSoldiers :=
		helper.IsBullish(opens[i2], closes[i2]) &&
			helper.IsBullish(opens[i1], closes[i1]) &&
			helper.IsBullish(opens[i], closes[i]) &&
			closes[i1] > closes[i2] && // each close is higher than previous
			closes[i] > closes[i1] &&
			opens[i] < opens[i2] && // each open stays above the open two bars ago
			opens[i1] > opens[i2] &&
			opens[i] < closes[i1] // current open stays inside previous body

	// 6. Inverted Hammer (Strength 6.0)
	isInvertedHammer := helper.IsBearish(opens[i], closes[i]) &&
		helper.IsSmallBody(opens[i], closes[i], highs[i], lows[i], atr, smallBodyAtrMul, 0.2) &&
		helper.UpperShadow(opens[i], closes[i], highs[i]) > 2*helper.BodySize(opens[i], closes[i]) &&
		helper.LowerShadow(opens[i], closes[i], lows[i]) < 0.1*helper.CandleRange(highs[i], lows[i])

	// 7. Bullish Harami (Strength 6.0)
	isHaramiBull :=
		helper.IsBearish(opens[i1], closes[i1]) &&
			helper.IsLongBody(opens[i1], closes[i1], highs[i1], lows[i1], atr, longBodyAtrMul, 0.6) &&
			helper.IsBullish(opens[i], closes[i]) &&
			helper.IsSmallBody(opens[i], closes[i], highs[i], lows[i], atr, smallBodyAtrMul, 0.25) &&
			helper.IsHaramiStrict(opens[i], closes[i], highs[i], lows[i],
				opens[i1], closes[i1], highs[i1], lows[i1])

	// 8. Rising Three (Strength 8.0)
	isRisingThree :=
		helper.IsBullish(opens[i4], closes[i4]) &&
			helper.IsLongBody(opens[i4], closes[i4], highs[i4], lows[i4], atr, longBodyAtrMul, 0.6) &&
			helper.IsBearish(opens[i3], closes[i3]) &&
			helper.IsBearish(opens[i2], closes[i2]) &&
			helper.IsBearish(opens[i1], closes[i1]) &&
			closes[i1] > opens[i4] && // the first bullish candle is higher than the low‑bearish block
			helper.IsBullish(opens[i], closes[i]) &&
			helper.IsLongBody(opens[i], closes[i], highs[i], lows[i], atr, longBodyAtrMul, 0.6) &&
			closes[i] > highs[i4]

	// 9. Tweezer Bottom (Strength 6.0)
	isTweezersBottom :=
		lows[i1] == lows[i] && // equal lows on two consecutive bars
			helper.IsBearish(opens[i1], closes[i1]) && // first bar is bearish
			helper.IsBullish(opens[i], closes[i]) // second bar bullish

	// 10. Bullish Marubozu (Strength 8.0)
	isMarubozuBull :=
		helper.IsMarubozu(opens[i], closes[i], highs[i], lows[i], 0.05) && // body occupies >95 % of range
			helper.IsBullish(opens[i], closes[i])

	// 11. Belt Hold Bull (Strength 6.0)
	isBeltHoldBull :=
		helper.IsBullish(opens[i], closes[i]) &&
			helper.IsLongBody(opens[i], closes[i], highs[i], lows[i], atr, longBodyAtrMul, 0.6) &&
			opens[i] == lows[i] // opens at the low of the candle

	// 12. Matching Low (Strength 4.0)
	isMatchingLow :=
		helper.IsBearish(opens[i1], closes[i1]) &&
			helper.IsSmallBody(opens[i1], closes[i1], highs[i1], lows[i1], atr, smallBodyAtrMul, 0.2) &&
			helper.IsBearish(opens[i], closes[i]) &&
			helper.IsSmallBody(opens[i], closes[i], highs[i], lows[i], atr, smallBodyAtrMul, 0.2) &&
			closes[i] == closes[i1] && // identical closes
			lows[i] == lows[i1] // identical lows

	// 13. Three Inside Up (Strength 7.0)
	is3InsideBull :=
		helper.IsBearish(opens[i2], closes[i2]) &&
			helper.IsLongBody(opens[i2], closes[i2], highs[i2], lows[i2], atr, longBodyAtrMul, 0.6) &&
			// The middle candle must be a bullish harami relative to the first
			isHaramiBull && // we already computed it for bar i1
			helper.IsBullish(opens[i], closes[i]) &&
			closes[i] > closes[i2]

	// 14. Kicking Bull (Strength 10.0)
	isKickingBull :=
		helper.IsMarubozu(opens[i1], closes[i1], highs[i1], lows[i1], 0.05) && // first candle is a marubozu
			helper.IsBearish(opens[i1], closes[i1]) && // first candle bearish
			isMarubozuBull && // second candle is bullish marubozu
			helper.IsGapUp(opens[i], highs[i1]) // gap up between them

	// 15. Stick Sandwich (Strength 6.0)
	isStickSandwich :=
		helper.IsBearish(opens[i2], closes[i2]) && // two consecutive bearish candles
			helper.IsBearish(opens[i1], closes[i1]) &&
			helper.IsBullish(opens[i], closes[i]) && // third candle bullish
			closes[i] == closes[i2] && // close equals the close of the first candle
			closes[i] < closes[i1] // but lies below the close of the middle candle

	// 16. Ladder Bottom (Strength 7.0)
	isLadderBottom :=
		helper.IsBearish(opens[i4], closes[i4]) &&
			helper.IsBearish(opens[i3], closes[i3]) &&
			helper.IsBearish(opens[i2], closes[i2]) &&
			helper.IsBearish(opens[i1], closes[i1]) &&
			helper.IsSmallBody(opens[i1], closes[i1], highs[i1], lows[i1], atr, smallBodyAtrMul, 0.2) &&
			opens[i1] < closes[i2] && // middle candle opens inside previous body
			helper.IsBullish(opens[i], closes[i]) &&
			helper.IsLongBody(opens[i], closes[i], highs[i], lows[i], atr, longBodyAtrMul, 0.6)

	// 17. Dragonfly Doji (Strength 8.0)
	isDragonflyDoji :=
		isDoji &&
			helper.LowerShadow(opens[i], closes[i], lows[i]) > 5*helper.BodySize(opens[i], closes[i]) && // long lower shadow
			helper.UpperShadow(opens[i], closes[i], highs[i]) < 0.1*helper.BodySize(opens[i], closes[i])

	// 18. White Marubozu (Strength 8.0)
	//   (same definition as a bullish marubozu, already computed)
	isWhiteMarubozu := isMarubozuBull

	// 19. Three Line Strike Bull (Strength 9.0)
	is3LineStrikeBull :=
		helper.IsBearish(opens[i3], closes[i3]) &&
			helper.IsBearish(opens[i2], closes[i2]) &&
			helper.IsBearish(opens[i1], closes[i1]) &&
			closes[i1] < closes[i2] && // each close lower than the next
			closes[i2] < closes[i3] &&
			helper.IsBullish(opens[i], closes[i]) &&
			helper.IsLongBody(opens[i], closes[i], highs[i], lows[i], atr, longBodyAtrMul, 0.6) &&
			closes[i] > opens[i3] // final bullish candle closes above the open of the first bearish candle

	// 20. Abandoned Baby Bull (Strength 10.0)
	isAbandonedBabyBull :=
		helper.IsBearish(opens[i2], closes[i2]) &&
			helper.IsLongBody(opens[i2], closes[i2], highs[i2], lows[i2], atr, longBodyAtrMul, 0.6) &&
			// The middle candle must be a doji (the script uses `isDojiCandle[…]` on bar i‑1)
			helper.IsDoji(opens[i1], closes[i1], highs[i1], lows[i1], atr, smallBodyAtrMul, 0.1) &&
			lows[i1] < lows[i2] && // doji’s low below prior low
			highs[i1] < closes[i2] && // doji’s high below prior close
			helper.IsBullish(opens[i], closes[i]) &&
			helper.IsLongBody(opens[i], closes[i], highs[i], lows[i], atr, longBodyAtrMul, 0.6) &&
			opens[i] > highs[i1] && // bullish candle opens above doji high
			opens[i] > closes[i2]

	// 21. Thrusting Line (Strength 4.0)
	isThrustingLine :=
		helper.IsBearish(opens[i1], closes[i1]) &&
			helper.IsLongBody(opens[i1], closes[i1], highs[i1], lows[i1], atr, longBodyAtrMul, 0.6) &&
			helper.IsBullish(opens[i], closes[i]) &&
			opens[i] < closes[i1] && // opens inside prior body
			closes[i] > closes[i1] && // closes above prior close
			closes[i] < (opens[i1]+closes[i1])/2 // but not above the midpoint of the prior body

	// 22. Meeting Lines Bull (Strength 4.0)
	isMeetingLinesBull :=
		helper.IsBearish(opens[i1], closes[i1]) &&
			helper.IsBullish(opens[i], closes[i]) &&
			math.Abs(closes[i]-closes[i1])/mintick < 2 // closes are practically equal

	// 23. Separating Lines Bull (Strength 6.0)
	isSeparatingLinesBull :=
		helper.IsBullish(opens[i1], closes[i1]) && // previous bar bullish
			helper.IsLongBody(opens[i1], closes[i1], highs[i1], lows[i1], atr, longBodyAtrMul, 0.6) &&
			helper.IsBullish(opens[i], closes[i]) && // current bar bullish
			helper.IsLongBody(opens[i], closes[i], highs[i], lows[i], atr, longBodyAtrMul, 0.6) &&
			opens[i] == opens[i1] && // opens are exactly equal
			opens[i] > closes[i1] // open is above prior close (upward gap)

	// 24. Unique Three River Bottom (Strength 8.0)
	isUnique3RiverBottom :=
		helper.IsBearish(opens[i2], closes[i2]) &&
			helper.IsBearish(opens[i1], closes[i1]) &&
			lows[i1] < lows[i2] && // lower low on the middle candle
			helper.IsBullish(opens[i], closes[i]) &&
			opens[i] > lows[i1] && // open above the low of the middle candle
			opens[i] < closes[i1] && // open still inside the prior body
			closes[i] < opens[i1] && // close below prior open
			lows[i] == lows[i1] // low equal to middle low (the “river”)

	// 25. Hook Reversal Bull (Strength 6.0)
	isHookReversalBull :=
		helper.IsBullish(opens[i1], closes[i1]) && // previous bar bullish
			helper.IsLongBody(opens[i1], closes[i1], highs[i1], lows[i1], atr, longBodyAtrMul, 0.6) &&
			helper.IsBullish(opens[i], closes[i]) &&
			helper.IsSmallBody(opens[i], closes[i], highs[i], lows[i], atr, smallBodyAtrMul, 0.25) &&
			opens[i] > closes[i1] && // opens above prior close
			opens[i] < opens[i1] && // but below prior open (a “hook”)
			closes[i] > highs[i1] // closes above prior high

	// --------------------------------------------------------------
	// Bearish patterns – analogous to the bullish ones
	// --------------------------------------------------------------

	// 1. Hanging Man (Strength 8.0)
	isHangingMan := helper.IsBullish(opens[i], closes[i]) &&
		helper.IsSmallBody(opens[i], closes[i], highs[i], lows[i], atr, smallBodyAtrMul, 0.2) &&
		helper.LowerShadow(opens[i], closes[i], lows[i]) > 2*helper.BodySize(opens[i], closes[i]) &&
		helper.UpperShadow(opens[i], closes[i], highs[i]) < 0.1*helper.CandleRange(highs[i], lows[i])

	// 2. Bearish Engulfing (Strength 10.0)
	isBearishEngulf := helper.IsBullish(opens[i1], closes[i1]) &&
		helper.IsBearish(opens[i], closes[i]) &&
		helper.IsEngulfing(opens[i], closes[i], opens[i1], closes[i1])

	// 3. Dark Cloud Cover (Strength 8.0)
	isDarkCloudCover := helper.IsBullish(opens[i1], closes[i1]) &&
		helper.IsLongBody(opens[i1], closes[i1], highs[i1], lows[i1], atr, longBodyAtrMul, 0.6) &&
		helper.IsBearish(opens[i], closes[i]) &&
		opens[i] > highs[i1] && // open gaps above prior high
		closes[i] < (opens[i1]+closes[i1])/2 && // close below midpoint of prior body
		closes[i] > opens[i1] // but still above prior open

	// 4. Evening Star (Strength 7.0)
	isEveningStar :=
		helper.IsBullish(opens[i2], closes[i2]) &&
			helper.IsLongBody(opens[i2], closes[i2], highs[i2], lows[i2], atr, longBodyAtrMul, 0.6) &&
			helper.IsSmallBody(opens[i1], closes[i1], highs[i1], lows[i1], atr, smallBodyAtrMul, 0.2) &&
			opens[i1] > closes[i2] && // middle candle opens above prior close
			helper.IsBearish(opens[i], closes[i]) &&
			helper.IsLongBody(opens[i], closes[i], highs[i], lows[i], atr, longBodyAtrMul, 0.6) &&
			closes[i] < (opens[i2]+closes[i2])/2 // close below midpoint of the first candle

	// 5. Three Black Crows (Strength 9.0)
	is3BlackCrows :=
		helper.IsBearish(opens[i2], closes[i2]) &&
			helper.IsBearish(opens[i1], closes[i1]) &&
			helper.IsBearish(opens[i], closes[i]) &&
			closes[i1] < closes[i2] && // each close lower than previous
			closes[i] < closes[i1] &&
			opens[i] < opens[i2] && // each open stays below open two bars ago
			opens[i1] < opens[i2] &&
			opens[i] > closes[i1] // current open stays inside previous body

	// 6. Gravestone Doji (Strength 8.0)
	isGravestoneDoji :=
		isDoji &&
			helper.UpperShadow(opens[i], closes[i], highs[i]) > 5*helper.BodySize(opens[i], closes[i]) && // long upper shadow
			helper.LowerShadow(opens[i], closes[i], lows[i]) < 0.1*helper.BodySize(opens[i], closes[i])

	// 7. Shooting Star (Strength 6.0)
	isShootingStar :=
		helper.IsBullish(opens[i], closes[i]) &&
			helper.IsSmallBody(opens[i], closes[i], highs[i], lows[i], atr, smallBodyAtrMul, 0.2) &&
			helper.UpperShadow(opens[i], closes[i], highs[i]) > 2*helper.BodySize(opens[i], closes[i]) &&
			helper.LowerShadow(opens[i], closes[i], lows[i]) < 0.1*helper.CandleRange(highs[i], lows[i])

	// 8. Bearish Harami (Strength 6.0)
	isHaramiBear :=
		helper.IsBullish(opens[i1], closes[i1]) &&
			helper.IsLongBody(opens[i1], closes[i1], highs[i1], lows[i1], atr, longBodyAtrMul, 0.6) &&
			helper.IsBearish(opens[i], closes[i]) &&
			helper.IsSmallBody(opens[i], closes[i], highs[i], lows[i], atr, smallBodyAtrMul, 0.25) &&
			helper.IsHaramiStrict(opens[i], closes[i], highs[i], lows[i],
				opens[i1], closes[i1], highs[i1], lows[i1])

	// 9. Falling Three (Strength 8.0)
	isFallingThree :=
		helper.IsBearish(opens[i4], closes[i4]) &&
			helper.IsLongBody(opens[i4], closes[i4], highs[i4], lows[i4], atr, longBodyAtrMul, 0.6) &&
			helper.IsBullish(opens[i3], closes[i3]) &&
			helper.IsBullish(opens[i2], closes[i2]) &&
			helper.IsBullish(opens[i1], closes[i1]) &&
			closes[i1] < opens[i4] && // the first bullish candle is lower than the low‑bearish block
			helper.IsBearish(opens[i], closes[i]) &&
			helper.IsLongBody(opens[i], closes[i], highs[i], lows[i], atr, longBodyAtrMul, 0.6) &&
			closes[i] < lows[i4]

	// 10. Tweezer Top (Strength 6.0)
	isTweezersTop :=
		highs[i1] == highs[i] && // equal highs on two consecutive bars
			helper.IsBullish(opens[i1], closes[i1]) &&
			helper.IsBearish(opens[i], closes[i])

	// 11. Bearish Marubozu (Strength 8.0)
	isMarubozuBear :=
		helper.IsMarubozu(opens[i], closes[i], highs[i], lows[i], 0.05) && // body >95 % of range
			helper.IsBearish(opens[i], closes[i])

	// 12. Belt Hold Bear (Strength 6.0)
	isBeltHoldBear :=
		helper.IsBearish(opens[i], closes[i]) &&
			helper.IsLongBody(opens[i], closes[i], highs[i], lows[i], atr, longBodyAtrMul, 0.6) &&
			opens[i] == highs[i] // opens at the high of the candle

	// 13. Matching High (Strength 4.0)
	isMatchingHigh :=
		helper.IsBullish(opens[i1], closes[i1]) &&
			helper.IsSmallBody(opens[i1], closes[i1], highs[i1], lows[i1], atr, smallBodyAtrMul, 0.2) &&
			helper.IsBullish(opens[i], closes[i]) &&
			helper.IsSmallBody(opens[i], closes[i], highs[i], lows[i], atr, smallBodyAtrMul, 0.2) &&
			closes[i] == closes[i1] && // identical closes
			highs[i] == highs[i1] // identical highs

	// 14. Three Inside Down (Strength 7.0)
	is3InsideBear :=
		helper.IsBullish(opens[i2], closes[i2]) &&
			helper.IsLongBody(opens[i2], closes[i2], highs[i2], lows[i2], atr, longBodyAtrMul, 0.6) &&
			// middle candle must be a bearish harami relative to the first
			isHaramiBear && // we already computed it for bar i1
			helper.IsBearish(opens[i], closes[i]) &&
			closes[i] < closes[i2]

	// 15. Kicking Bear (Strength 10.0)
	isKickingBear :=
		helper.IsMarubozu(opens[i1], closes[i1], highs[i1], lows[i1], 0.05) && // first candle marubozu
			helper.IsBullish(opens[i1], closes[i1]) && // first candle bullish
			isMarubozuBear && // second candle bearish marubozu
			helper.IsGapDown(opens[i], lows[i1]) // gap down between them

	// 16. Deliberation (Strength 4.0)
	isDeliberation :=
		helper.IsBullish(opens[i2], closes[i2]) &&
			helper.IsBullish(opens[i1], closes[i1]) &&
			helper.IsSmallBody(opens[i1], closes[i1], highs[i1], lows[i1], atr, smallBodyAtrMul, 0.2) &&
			opens[i] > opens[i2] && // opens higher than the candle two bars ago
			closes[i] > closes[i2] && // closes higher than the candle two bars ago
			helper.IsSmallBody(opens[i], closes[i], highs[i], lows[i], atr, smallBodyAtrMul, 0.2)

	// 17. Descending Hawk (Strength 4.0)
	isDescendingHawk :=
		helper.IsBullish(opens[i1], closes[i1]) &&
			helper.IsBearish(opens[i], closes[i]) &&
			highs[i] == highs[i1] && // equal highs
			lows[i] < lows[i1] // lower low

	// 18. Downside Tasuki Gap (Strength 6.0)
	isDownsideTasukiGap :=
		helper.IsBearish(opens[i2], closes[i2]) &&
			helper.IsLongBody(opens[i2], closes[i2], highs[i2], lows[i2], atr, longBodyAtrMul, 0.6) &&
			helper.IsBearish(opens[i1], closes[i1]) &&
			helper.IsLongBody(opens[i1], closes[i1], highs[i1], lows[i1], atr, longBodyAtrMul, 0.6) &&
			opens[i1] < closes[i2] && // second candle gaps down from first
			helper.IsBullish(opens[i], closes[i]) &&
			helper.IsSmallBody(opens[i], closes[i], highs[i], lows[i], atr, smallBodyAtrMul, 0.2) &&
			opens[i] > closes[i1] && // bullish candle opens above prior close
			opens[i] < opens[i1] && // but still inside the gap
			closes[i] > opens[i1] && // and closes above the prior open
			closes[i] < closes[i2]

	// 19. Upside Gap Two Crows (Strength 8.0)
	isUpsideGapTwoCrows :=
		helper.IsBullish(opens[i2], closes[i2]) &&
			helper.IsLongBody(opens[i2], closes[i2], highs[i2], lows[i2], atr, longBodyAtrMul, 0.6) &&
			helper.IsBearish(opens[i1], closes[i1]) &&
			helper.IsSmallBody(opens[i1], closes[i1], highs[i1], lows[i1], atr, smallBodyAtrMul, 0.2) &&
			opens[i1] > highs[i2] && // gap up between first and second candle
			helper.IsBearish(opens[i], closes[i]) &&
			closes[i] < opens[i1] && // third candle closes below second open
			opens[i] < closes[i2] // third candle opens below first close

	// 20. Black Marubozu (Strength 8.0) – same as bearish marubozu
	isBlackMarubozu := isMarubozuBear

	// 21. Dark Cloud Cover (Weakened) (Strength 6.0)
	isDarkCloudCoverWeak :=
		helper.IsBullish(opens[i1], closes[i1]) &&
			helper.IsLongBody(opens[i1], closes[i1], highs[i1], lows[i1], atr, longBodyAtrMul, 0.6) &&
			helper.IsBearish(opens[i], closes[i]) &&
			helper.IsLongBody(opens[i], closes[i], highs[i], lows[i], atr, longBodyAtrMul, 0.6) &&
			opens[i] > highs[i1] && // open gaps above prior high
			closes[i] <= (opens[i1]+closes[i1])/2 && // close ≤ 50 % of prior body
			closes[i] > 0.75*opens[i1]+0.25*closes[i1] // close > 75 %‑weighted average (as in the script)

	// 22. Meeting Lines Bear (Strength 4.0)
	isMeetingLinesBear :=
		helper.IsBullish(opens[i1], closes[i1]) &&
			helper.IsBearish(opens[i], closes[i]) &&
			math.Abs(closes[i]-closes[i1])/mintick < 2 // closes nearly equal

	// 23. Separating Lines Bear (Strength 6.0)
	isSeparatingLinesBear :=
		helper.IsBearish(opens[i1], closes[i1]) && // previous bar bearish
			helper.IsLongBody(opens[i1], closes[i1], highs[i1], lows[i1], atr, longBodyAtrMul, 0.6) &&
			helper.IsBearish(opens[i], closes[i]) && // current bar bearish
			helper.IsLongBody(opens[i], closes[i], highs[i], lows[i], atr, longBodyAtrMul, 0.6) &&
			opens[i] == opens[i1] && // opens exactly equal
			opens[i] < closes[i1] // open below prior close (downward gap)

	// 24. Concealing Baby Swallow (Strength 8.0)
	isConcealingBabySwallow :=
		helper.IsMarubozu(opens[i3], closes[i3], highs[i3], lows[i3], 0.05) && // three consecutive marubozus
			helper.IsBearish(opens[i3], closes[i3]) &&
			helper.IsMarubozu(opens[i2], closes[i2], highs[i2], lows[i2], 0.05) &&
			helper.IsBearish(opens[i2], closes[i2]) &&
			helper.IsBearish(opens[i1], closes[i1]) && // third candle bearish (no body check needed)
			opens[i1] < lows[i2] && // open below prior low (the “concealing” gap)
			helper.IsBearish(opens[i], closes[i]) &&
			helper.IsLongBody(opens[i], closes[i], highs[i], lows[i], atr, longBodyAtrMul, 0.6) &&
			opens[i] < closes[i1] // final bearish candle closes below prior close

	// 25. Hook Reversal Bear (Strength 6.0)
	isHookReversalBear :=
		helper.IsBullish(opens[i1], closes[i1]) && // previous bar bullish
			helper.IsLongBody(opens[i1], closes[i1], highs[i1], lows[i1], atr, longBodyAtrMul, 0.6) &&
			helper.IsBearish(opens[i], closes[i]) &&
			helper.IsSmallBody(opens[i], closes[i], highs[i], lows[i], atr, smallBodyAtrMul, 0.25) &&
			opens[i] < closes[i1] && // opens below prior close
			opens[i] > opens[i1] && // but above prior open (the “hook”)
			closes[i] < lows[i1] // closes below prior low

	// ---------------------------------------------------------------------
	//                     NEUTRAL (INDECISION) PATTERN BOOLEANS
	// ---------------------------------------------------------------------

	// 2. Long‑Legged Doji (Strength 4.0)
	isLongLeggedDoji :=
		isDoji &&
			// at least one shadow ≥ 30 % of the total range
			(helper.IsLongShadowPercent(helper.UpperShadow(opens[i], closes[i], highs[i]),
				helper.CandleRange(highs[i], lows[i]), 0.3) ||
				helper.IsLongShadowPercent(helper.LowerShadow(opens[i], closes[i], lows[i]),
					helper.CandleRange(highs[i], lows[i]), 0.3))

	// 3. Four‑Price Doji (Strength 2.0) – OHLC are exactly equal
	isFourPriceDoji :=
		opens[i] == highs[i] && opens[i] == lows[i] && opens[i] == closes[i]

	// 4. Spinning Top (Strength 4.0)
	isSpinningTop :=
		helper.IsSmallBody(opens[i], closes[i], highs[i], lows[i],
			atr, smallBodyAtrMul, 0.25) &&
			// both shadows ≥ 20 % of the total range
			helper.IsLongShadowPercent(helper.UpperShadow(opens[i], closes[i], highs[i]),
				helper.CandleRange(highs[i], lows[i]), 0.2) &&
			helper.IsLongShadowPercent(helper.LowerShadow(opens[i], closes[i], lows[i]),
				helper.CandleRange(highs[i], lows[i]), 0.2)

	// 5. Gapping Doji (Strength 4.0) – a doji that gaps up or down from the previous candle
	isGappingDoji :=
		isDoji &&
			(helper.IsGapUp(opens[i], highs[i1]) || helper.IsGapDown(opens[i], lows[i1]))

	// 6. Harami Cross (Bullish) (Strength 8.0)
	isHaramiCrossBull :=
		helper.IsBullish(opens[i1], closes[i1]) && // previous bullish candle
			helper.IsLongBody(opens[i1], closes[i1], highs[i1], lows[i1],
				atr, longBodyAtrMul, 0.6) &&
			isDoji && // current candle is a doji
			helper.IsHaramiStrict(opens[i], closes[i], highs[i], lows[i],
				opens[i1], closes[i1], highs[i1], lows[i1])

	// 7. Harami Cross (Bearish) (Strength 8.0)
	isHaramiCrossBear :=
		helper.IsBearish(opens[i1], closes[i1]) && // previous bearish candle
			helper.IsLongBody(opens[i1], closes[i1], highs[i1], lows[i1],
				atr, longBodyAtrMul, 0.6) &&
			isDoji && // current candle is a doji
			helper.IsHaramiStrict(opens[i], closes[i], highs[i], lows[i],
				opens[i1], closes[i1], highs[i1], lows[i1])

	// 8. Upside Tasuki Gap (Neutral) (Strength 4.0)
	isUpsideTasukiGap :=
		// first bullish candle
		helper.IsBullish(opens[i2], closes[i2]) &&
			// second bullish candle that gaps up
			helper.IsBullish(opens[i1], closes[i1]) &&
			opens[i1] > highs[i2] && // gap up
			// third candle (bearish) that closes within the gap
			helper.IsBearish(opens[i], closes[i]) &&
			helper.IsSmallBody(opens[i], closes[i], highs[i], lows[i],
				atr, smallBodyAtrMul, 0.25) &&
			opens[i] < closes[i1] && // open below previous close
			opens[i] > opens[i1] && // but still inside the gap
			closes[i] > opens[i1] && // close above previous open
			closes[i] < closes[i2] // close below first candle close

	// 9. On‑Neck Line (Neutral) (Strength 4.0)
	isOnNeckLine :=
		// prior bearish candle with long body
		helper.IsBearish(opens[i1], closes[i1]) &&
			helper.IsLongBody(opens[i1], closes[i1], highs[i1], lows[i1],
				atr, longBodyAtrMul, 0.6) &&
			// current bullish candle with very small body (doji‑like) near the prior low
			helper.IsBullish(opens[i], closes[i]) &&
			helper.IsSmallBody(opens[i], closes[i], highs[i], lows[i],
				atr, smallBodyAtrMul, 0.25) &&
			opens[i] < lows[i1] && // opens below prior low (gap down)
			math.Abs(closes[i]-closes[i1])/mintick < 2 // close within 2 ticks of prior close

	// 10. In‑Neck Line (Neutral) (Strength 4.0)
	isInNeckLine :=
		// prior bearish candle with long body
		helper.IsBearish(opens[i1], closes[i1]) &&
			helper.IsLongBody(opens[i1], closes[i1], highs[i1], lows[i1],
				atr, longBodyAtrMul, 0.6) &&
			// current bullish candle, again tiny body
			helper.IsBullish(opens[i], closes[i]) &&
			helper.IsSmallBody(opens[i], closes[i], highs[i], lows[i],
				atr, smallBodyAtrMul, 0.25) &&
			opens[i] < lows[i1] && // opens below prior low
			closes[i] > closes[i1] && // closes above prior close
			// final condition: close is within 10 % of the prior body length
			closes[i] < closes[i1]+helper.BodySize(opens[i1], closes[i1])*0.1

	// 11. Three‑Bar Inside Bull (Neutral) (Strength 3.0)
	is3BarInsideBull :=
		// first bullish candle
		helper.IsBullish(opens[i2], closes[i2]) &&
			// second bullish candle that is fully inside the first candle's range
			helper.IsBullish(opens[i1], closes[i1]) &&
			opens[i1] > opens[i2] && closes[i1] < closes[i2] && // inside
			// third bullish candle (any size – we only need the direction)
			helper.IsBullish(opens[i], closes[i])

	// 12. Three‑Bar Inside Bear (Neutral) (Strength 3.0)
	is3BarInsideBear :=
		helper.IsBearish(opens[i2], closes[i2]) &&
			helper.IsBearish(opens[i1], closes[i1]) &&
			opens[i1] < opens[i2] && closes[i1] > closes[i2] && // inside
			helper.IsBearish(opens[i], closes[i])

	// 13. Homing Pigeon (Neutral) (Strength 6.0)
	isHomingPigeon :=
		// prior bearish long candle
		helper.IsBearish(opens[i1], closes[i1]) &&
			helper.IsLongBody(opens[i1], closes[i1], highs[i1], lows[i1],
				atr, longBodyAtrMul, 0.6) &&
			// current bullish candle that is a small‑body harami inside the previous candle
			helper.IsBullish(opens[i], closes[i]) &&
			helper.IsSmallBody(opens[i], closes[i], highs[i], lows[i],
				atr, smallBodyAtrMul, 0.25) &&
			helper.IsHaramiStrict(opens[i], closes[i], highs[i], lows[i],
				opens[i1], closes[i1], highs[i1], lows[i1])

	// 14. Last Engulfing Bottom (Neutral) (Strength 4.0)
	isLastEngulfingBottom :=
		// immediate previous bullish candle (any size)
		helper.IsBullish(opens[i1], closes[i1]) &&
			// current bearish candle that engulfs the prior bullish candle
			helper.IsBearish(opens[i], closes[i]) &&
			helper.IsEngulfing(opens[i], closes[i], opens[i1], closes[i1])

	// 15. Last Engulfing Top (Neutral) (Strength 4.0)
	isLastEngulfingTop :=
		helper.IsBearish(opens[i1], closes[i1]) &&
			helper.IsBullish(opens[i], closes[i]) &&
			helper.IsEngulfing(opens[i], closes[i], opens[i1], closes[i1])

	// 16. Counterattack Bull (Neutral) (Strength 4.0)
	isCounterattackBull :=
		// prior bearish long candle
		helper.IsBearish(opens[i1], closes[i1]) &&
			helper.IsLongBody(opens[i1], closes[i1], highs[i1], lows[i1],
				atr, longBodyAtrMul, 0.6) &&
			// current bullish candle that closes at the same price (within 2 ticks)
			helper.IsBullish(opens[i], closes[i]) &&
			helper.IsLongBody(opens[i], closes[i], highs[i], lows[i],
				atr, longBodyAtrMul, 0.6) &&
			math.Abs(closes[i]-closes[i1])/mintick < 2

	// 17. Counterattack Bear (Neutral) (Strength 4.0)
	isCounterattackBear :=
		helper.IsBullish(opens[i1], closes[i1]) &&
			helper.IsLongBody(opens[i1], closes[i1], highs[i1], lows[i1],
				atr, longBodyAtrMul, 0.6) &&
			helper.IsBearish(opens[i], closes[i]) &&
			helper.IsLongBody(opens[i], closes[i], highs[i], lows[i],
				atr, longBodyAtrMul, 0.6) &&
			math.Abs(closes[i]-closes[i1])/mintick < 2

	// 18. Three Stars in the South (Neutral) (Strength 4.0)
	// (All three candles are tiny dojis with long shadows)
	isThreeStarsSouth :=
		helper.IsDoji(opens[i2], closes[i2], highs[i2], lows[i2],
			atr, smallBodyAtrMul, 0.1) &&
			helper.IsDoji(opens[i1], closes[i1], highs[i1], lows[i1],
				atr, smallBodyAtrMul, 0.1) &&
			isDoji &&
			// each candle’s upper shadow ≥ 30 % of total range
			helper.IsLongShadowPercent(helper.UpperShadow(opens[i2], closes[i2], highs[i2]),
				helper.CandleRange(highs[i2], lows[i2]), 0.3) &&
			helper.IsLongShadowPercent(helper.UpperShadow(opens[i1], closes[i1], highs[i1]),
				helper.CandleRange(highs[i1], lows[i1]), 0.3) &&
			helper.IsLongShadowPercent(helper.UpperShadow(opens[i], closes[i], highs[i]),
				helper.CandleRange(highs[i], lows[i]), 0.3)

	// 19. Three Stars in the North (Neutral) (Strength 4.0)
	// (Same as the South version but with long lower shadows)
	isThreeStarsNorth :=
		helper.IsDoji(opens[i2], closes[i2], highs[i2], lows[i2],
			atr, smallBodyAtrMul, 0.1) &&
			helper.IsDoji(opens[i1], closes[i1], highs[i1], lows[i1],
				atr, smallBodyAtrMul, 0.1) &&
			isDoji &&
			helper.IsLongShadowPercent(helper.LowerShadow(opens[i2], closes[i2], lows[i2]),
				helper.CandleRange(highs[i2], lows[i2]), 0.3) &&
			helper.IsLongShadowPercent(helper.LowerShadow(opens[i1], closes[i1], lows[i1]),
				helper.CandleRange(highs[i1], lows[i1]), 0.3) &&
			helper.IsLongShadowPercent(helper.LowerShadow(opens[i], closes[i], lows[i]),
				helper.CandleRange(highs[i], lows[i]), 0.3)

	// 20. Squeeze Alert (Neutral) (Strength 2.0)
	// Very small body + both shadows ≥ 40 % of range
	isSqueezeAlert :=
		helper.IsSmallBody(opens[i], closes[i], highs[i], lows[i],
			atr, smallBodyAtrMul, 0.1) &&
			helper.IsLongShadowPercent(helper.UpperShadow(opens[i], closes[i], highs[i]),
				helper.CandleRange(highs[i], lows[i]), 0.4) &&
			helper.IsLongShadowPercent(helper.LowerShadow(opens[i], closes[i], lows[i]),
				helper.CandleRange(highs[i], lows[i]), 0.4)

	// 21. Stalled Pattern (Neutral) (Strength 4.0)
	isStalledPattern :=
		// three consecutive bullish long‑body candles
		helper.IsBullish(opens[i2], closes[i2]) &&
			helper.IsLongBody(opens[i2], closes[i2], highs[i2], lows[i2],
				atr, longBodyAtrMul, 0.6) &&
			helper.IsBullish(opens[i1], closes[i1]) &&
			helper.IsLongBody(opens[i1], closes[i1], highs[i1], lows[i1],
				atr, longBodyAtrMul, 0.6) &&
			helper.IsBullish(opens[i], closes[i]) &&
			// the current candle has a *small* body & a long upper shadow (price stalled upward)
			helper.IsSmallBody(opens[i], closes[i], highs[i], lows[i],
				atr, smallBodyAtrMul, 0.15) &&
			helper.IsLongShadowPercent(helper.UpperShadow(opens[i], closes[i], highs[i]),
				helper.CandleRange(highs[i], lows[i]), 0.5)

	// 22. Upside‑Downside Gap Three Bull (Neutral) (Strength 8.0)
	isUpsideDownsideGap3Bull :=
		helper.IsBullish(opens[i3], closes[i3]) &&
			helper.IsLongBody(opens[i3], closes[i3], highs[i3], lows[i3],
				atr, longBodyAtrMul, 0.6) &&
			// two successive gaps up
			helper.IsBullish(opens[i2], closes[i2]) &&
			opens[i2] > highs[i3] && // first gap up
			helper.IsBullish(opens[i1], closes[i1]) &&
			opens[i1] > highs[i2] && // second gap up
			// final bullish candle that closes above the high of the first candle
			helper.IsBullish(opens[i], closes[i]) &&
			closes[i] > highs[i3]

	// 23. Upside‑Downside Gap Three Bear (Neutral) (Strength 8.0)
	isUpsideDownsizeGap3Bear :=
		helper.IsBearish(opens[i3], closes[i3]) &&
			helper.IsLongBody(opens[i3], closes[i3], highs[i3], lows[i3],
				atr, longBodyAtrMul, 0.6) &&
			// two successive gaps down
			helper.IsBearish(opens[i2], closes[i2]) &&
			opens[i2] < lows[i3] && // first gap down
			helper.IsBearish(opens[i1], closes[i1]) &&
			opens[i1] < lows[i2] && // second gap down
			// final bearish candle that closes below the low of the first candle
			helper.IsBearish(opens[i], closes[i]) &&
			closes[i] < lows[i3]

	// 24. Engulfing Doji (Neutral) (Strength 6.0)
	// A doji that engulfs the previous candle (both sides of the doji extend beyond the prior body)
	isEngulfingDoji :=
		isDoji &&
			((opens[i] < opens[i1] && closes[i] > closes[i1]) || // bullish engulfing doji
				(opens[i] > opens[i1] && closes[i] < closes[i1])) && // bearish engulfing doji
			highs[i] > math.Max(opens[i1], closes[i1]) && // upper shadow exceeds prior high
			lows[i] < math.Min(opens[i1], closes[i1]) // lower shadow exceeds prior low

	// 25. High‑Wave Candle (Neutral) (Strength 4.0)
	// Small body, very long shadows on **both** sides
	isHighWaveCandle :=
		helper.IsSmallBody(opens[i], closes[i], highs[i], lows[i],
			atr, smallBodyAtrMul, 0.1) &&
			helper.IsLongShadowPercent(helper.UpperShadow(opens[i], closes[i], highs[i]),
				helper.CandleRange(highs[i], lows[i]), 0.3) &&
			helper.IsLongShadowPercent(helper.LowerShadow(opens[i], closes[i], lows[i]),
				helper.CandleRange(highs[i], lows[i]), 0.3)

	// 26. One‑Bar Reversal Bull (Neutral) (Strength 3.0)
	// Tiny body + closes **above** the prior high (gap up)
	isOneBarReversalBull :=
		helper.IsBullish(opens[i], closes[i]) &&
			helper.IsSmallBody(opens[i], closes[i], highs[i], lows[i],
				atr, smallBodyAtrMul, 0.1) &&
			opens[i] > highs[i1]

	// 27. One‑Bar Reversal Bear (Neutral) (Strength 3.0)
	// Tiny body + closes **below** the prior low (gap down)
	isOneBarReversalBear :=
		helper.IsBearish(opens[i], closes[i]) &&
			helper.IsSmallBody(opens[i], closes[i], highs[i], lows[i],
				atr, smallBodyAtrMul, 0.1) &&
			opens[i] < lows[i1]

	// 28. Three Gap Up (Neutral) (Strength 3.0)
	// Three consecutive bullish candles, each opening **above** the previous high
	isThreeGapUp :=
		helper.IsBullish(opens[i2], closes[i2]) && opens[i2] > highs[i3] &&
			helper.IsBullish(opens[i1], closes[i1]) && opens[i1] > highs[i2] &&
			helper.IsBullish(opens[i], closes[i]) && opens[i] > highs[i1]

	// 29. Three Gap Down (Neutral) (Strength 3.0)
	// Three consecutive bearish candles, each opening **below** the previous low
	isThreeGapDown :=
		helper.IsBearish(opens[i2], closes[i2]) && opens[i2] < lows[i3] &&
			helper.IsBearish(opens[i1], closes[i1]) && opens[i1] < lows[i2] &&
			helper.IsBearish(opens[i], closes[i]) && opens[i] < lows[i1]

	// 30. Two Crows (Neutral) (Strength 4.0)
	// Two bearish candles after a prior bullish long‑body candle
	isTwoCrows :=
		helper.IsBullish(opens[i2], closes[i2]) &&
			helper.IsLongBody(opens[i2], closes[i2], highs[i2], lows[i2],
				atr, longBodyAtrMul, 0.6) &&
			helper.IsBearish(opens[i1], closes[i1]) &&
			helper.IsSmallBody(opens[i1], closes[i1], highs[i1], lows[i1],
				atr, smallBodyAtrMul, 0.2) &&
			helper.IsBearish(opens[i], closes[i]) &&
			opens[i] > closes[i1] && // third candle opens above prior close
			opens[i] < opens[i1] && // but still inside the gap
			closes[i] < opens[i1] // and closes below the prior open

	// 31. Morning Doji Star (Neutral) (Strength 7.0)
	isMorningDojiStar :=
		// prior bearish long candle
		helper.IsBearish(opens[i2], closes[i2]) &&
			helper.IsLongBody(opens[i2], closes[i2], highs[i2], lows[i2],
				atr, longBodyAtrMul, 0.6) &&
			// middle doji that gaps down
			helper.IsDoji(opens[i1], closes[i1], highs[i1], lows[i1],
				atr, smallBodyAtrMul, 0.1) &&
			opens[i1] < lows[i2] && // gap down
			// final bullish candle that closes above the midpoint of the first candle
			helper.IsBullish(opens[i], closes[i]) &&
			opens[i] > opens[i1] && // open above the doji
			closes[i] > (opens[i2]+closes[i2])/2

	// 32. Evening Doji Star (Neutral) (Strength 7.0)
	isEveningDojiStar :=
		helper.IsBullish(opens[i2], closes[i2]) &&
			helper.IsLongBody(opens[i2], closes[i2], highs[i2], lows[i2],
				atr, longBodyAtrMul, 0.6) &&
			helper.IsDoji(opens[i1], closes[i1], highs[i1], lows[i1],
				atr, smallBodyAtrMul, 0.1) &&
			opens[i1] > highs[i2] && // gap up
			helper.IsBearish(opens[i], closes[i]) &&
			opens[i] < opens[i1] && // open below the doji
			closes[i] < (opens[i2]+closes[i2])/2

	// 33. Advancing Block (Neutral) (Strength 6.0)
	isAdvancingBlock :=
		// three consecutive bullish candles where each open is higher than the open two bars ago
		helper.IsBullish(opens[i2], closes[i2]) &&
			helper.IsBullish(opens[i1], closes[i1]) &&
			helper.IsBullish(opens[i], closes[i]) &&
			opens[i1] > opens[i2] && opens[i1] < closes[i2] &&
			opens[i] > opens[i1] && opens[i] < closes[i1] &&
			// the current candle’s body is smaller than the previous bullish body
			helper.BodySize(opens[i], closes[i]) < helper.BodySize(opens[i1], closes[i1])

	// 34. Kicking (Indecision) (Strength 5.0)
	// Either a bullish marubozu followed by a bearish marubozu with a gap up,
	// or the reverse (bearish → bullish) with a gap down.
	isKickingIndecision :=
		(isMarubozuBull && isMarubozuBear && helper.IsGapUp(opens[i], highs[i1])) ||
			(isMarubozuBear && isMarubozuBull && helper.IsGapDown(opens[i], lows[i1]))

	// --------------------------------------------------------------
	// 4️⃣  Fill the three pattern‑result arrays
	// --------------------------------------------------------------
	// The arrays have the same size as in the Pine‑Script (25 bullish, 25 bearish,
	// 34 neutral).  For brevity we only show a few entries; the rest follow the
	// exact same pattern.

	bullishPatternResults := make([]bool, 25)
	bullishPatternStrengths := make([]float64, 25)
	bullishPatternStrengthValues := []float64{
		8.0, 10.0, 8.0, 7.0, 9.0, 6.0, 6.0, 8.0, 6.0, 8.0,
		6.0, 4.0, 7.0, 10.0, 6.0, 7.0,
		8.0, 8.0, 9.0, 10.0, 4.0, 4.0, 6.0, 8.0, 6.0,
	}

	// Fill the booleans for the patterns we calculated above.
	// (Only the first few are shown – you would continue for all 25.)
	bullishPatternResults[0] = isHammer
	bullishPatternStrengths[0] = bullishPatternStrengthValues[0]

	bullishPatternResults[1] = isBullishEngulf
	bullishPatternStrengths[1] = bullishPatternStrengthValues[1]

	bullishPatternResults[2] = isPiercingLine
	bullishPatternStrengths[2] = bullishPatternStrengthValues[2]

	bullishPatternResults[3] = isMorningStar
	bullishPatternStrengths[3] = bullishPatternStrengthValues[3]

	bullishPatternResults[4] = is3WhiteSoldiers
	bullishPatternStrengths[4] = bullishPatternStrengthValues[4]

	bullishPatternResults[5] = isInvertedHammer
	bullishPatternStrengths[5] = bullishPatternStrengthValues[5]

	bullishPatternResults[6] = isHaramiBull
	bullishPatternStrengths[6] = bullishPatternStrengthValues[6]

	bullishPatternResults[7] = isRisingThree
	bullishPatternStrengths[7] = bullishPatternStrengthValues[7]

	bullishPatternResults[8] = isTweezersBottom
	bullishPatternStrengths[8] = bullishPatternStrengthValues[8]

	bullishPatternResults[9] = isMarubozuBull
	bullishPatternStrengths[9] = bullishPatternStrengthValues[9]

	bullishPatternResults[10] = isBeltHoldBull
	bullishPatternStrengths[10] = bullishPatternStrengthValues[10]

	bullishPatternResults[11] = isMatchingLow
	bullishPatternStrengths[11] = bullishPatternStrengthValues[11]

	bullishPatternResults[12] = is3InsideBull
	bullishPatternStrengths[12] = bullishPatternStrengthValues[12]

	bullishPatternResults[13] = isKickingBull
	bullishPatternStrengths[13] = bullishPatternStrengthValues[13]

	bullishPatternResults[14] = isStickSandwich
	bullishPatternStrengths[14] = bullishPatternStrengthValues[14]

	bullishPatternResults[15] = isLadderBottom
	bullishPatternStrengths[15] = bullishPatternStrengthValues[15]

	bullishPatternResults[16] = isDragonflyDoji
	bullishPatternStrengths[16] = bullishPatternStrengthValues[16]

	bullishPatternResults[17] = isWhiteMarubozu
	bullishPatternStrengths[17] = bullishPatternStrengthValues[17]

	bullishPatternResults[18] = is3LineStrikeBull
	bullishPatternStrengths[18] = bullishPatternStrengthValues[18]

	bullishPatternResults[19] = isAbandonedBabyBull
	bullishPatternStrengths[19] = bullishPatternStrengthValues[19]

	bullishPatternResults[20] = isThrustingLine
	bullishPatternStrengths[20] = bullishPatternStrengthValues[20]

	bullishPatternResults[21] = isMeetingLinesBull
	bullishPatternStrengths[21] = bullishPatternStrengthValues[21]

	bullishPatternResults[22] = isSeparatingLinesBull
	bullishPatternStrengths[22] = bullishPatternStrengthValues[22]

	bullishPatternResults[23] = isUnique3RiverBottom
	bullishPatternStrengths[23] = bullishPatternStrengthValues[23]

	bullishPatternResults[24] = isHookReversalBull
	bullishPatternStrengths[24] = bullishPatternStrengthValues[24]

	// ---- Bearish patterns -------------------------------------------------
	bearishPatternResults := make([]bool, 25) // will be filled later with the booleans
	bearishPatternStrengths := make([]float64, 25)

	bearishPatternStrengthValues := []float64{
		8.0, 10.0, 8.0, 7.0, 9.0, 8.0, 6.0, 6.0, 8.0,
		6.0, 8.0, 6.0, 4.0, 7.0, 10.0, 4.0, 4.0, 6.0,
		8.0, 8.0, 6.0, 4.0, 6.0, 8.0, 6.0,
	}

	bearishPatternResults[0] = isHangingMan
	bearishPatternStrengths[0] = bearishPatternStrengthValues[0]

	bearishPatternResults[1] = isBearishEngulf
	bearishPatternStrengths[1] = bearishPatternStrengthValues[1]

	bearishPatternResults[2] = isDarkCloudCover
	bearishPatternStrengths[2] = bearishPatternStrengthValues[2]

	bearishPatternResults[3] = isEveningStar
	bearishPatternStrengths[3] = bearishPatternStrengthValues[3]

	bearishPatternResults[4] = is3BlackCrows
	bearishPatternStrengths[4] = bearishPatternStrengthValues[4]

	bearishPatternResults[5] = isGravestoneDoji
	bearishPatternStrengths[5] = bearishPatternStrengthValues[5]

	bearishPatternResults[6] = isShootingStar
	bearishPatternStrengths[6] = bearishPatternStrengthValues[6]

	bearishPatternResults[7] = isHaramiBear
	bearishPatternStrengths[7] = bearishPatternStrengthValues[7]

	bearishPatternResults[8] = isFallingThree
	bearishPatternStrengths[8] = bearishPatternStrengthValues[8]

	bearishPatternResults[9] = isTweezersTop
	bearishPatternStrengths[9] = bearishPatternStrengthValues[9]

	bearishPatternResults[10] = isMarubozuBear
	bearishPatternStrengths[10] = bearishPatternStrengthValues[10]

	bearishPatternResults[11] = isBeltHoldBear
	bearishPatternStrengths[11] = bearishPatternStrengthValues[11]

	bearishPatternResults[12] = isMatchingHigh
	bearishPatternStrengths[12] = bearishPatternStrengthValues[12]

	bearishPatternResults[13] = is3InsideBear
	bearishPatternStrengths[13] = bearishPatternStrengthValues[13]

	bearishPatternResults[14] = isKickingBear
	bearishPatternStrengths[14] = bearishPatternStrengthValues[14]

	bearishPatternResults[15] = isDeliberation
	bearishPatternStrengths[15] = bearishPatternStrengthValues[15]

	bearishPatternResults[16] = isDescendingHawk
	bearishPatternStrengths[16] = bearishPatternStrengthValues[16]

	bearishPatternResults[17] = isDownsideTasukiGap
	bearishPatternStrengths[17] = bearishPatternStrengthValues[17]

	bearishPatternResults[18] = isUpsideGapTwoCrows
	bearishPatternStrengths[18] = bearishPatternStrengthValues[18]

	bearishPatternResults[19] = isBlackMarubozu
	bearishPatternStrengths[19] = bearishPatternStrengthValues[19]

	bearishPatternResults[20] = isDarkCloudCoverWeak
	bearishPatternStrengths[20] = bearishPatternStrengthValues[20]

	bearishPatternResults[21] = isMeetingLinesBear
	bearishPatternStrengths[21] = bearishPatternStrengthValues[21]

	bearishPatternResults[22] = isSeparatingLinesBear
	bearishPatternStrengths[22] = bearishPatternStrengthValues[22]

	bearishPatternResults[23] = isConcealingBabySwallow
	bearishPatternStrengths[23] = bearishPatternStrengthValues[23]

	bearishPatternResults[24] = isHookReversalBear
	bearishPatternStrengths[24] = bearishPatternStrengthValues[24]

	// ---------------------------------------------------------------
	//  Neutral (indecision) pattern containers – 34 entries
	// ---------------------------------------------------------------
	neutralPatternResults := make([]bool, 34)
	neutralPatternStrengths := make([]float64, 34)

	// Strengths in the exact order the patterns appear in the Pine‑Script
	neutralPatternStrengthValues := []float64{
		5.0, 4.0, 2.0, 4.0, 4.0, 8.0, 8.0, 4.0, 4.0, 4.0, 3.0, 3.0, 6.0, 4.0, 4.0, 4.0, 4.0, 2.0, 4.0, 2.0, 4.0, 8.0, 8.0, 6.0, 4.0, 3.0, 3.0, 3.0, 3.0, 4.0, 7.0, 7.0, 6.0, 5.0,
	}

	neutralPatternResults[0] = isDoji
	neutralPatternStrengths[0] = neutralPatternStrengthValues[0]

	neutralPatternResults[1] = isLongLeggedDoji
	neutralPatternStrengths[1] = neutralPatternStrengthValues[1]

	neutralPatternResults[2] = isFourPriceDoji
	neutralPatternStrengths[2] = neutralPatternStrengthValues[2]

	neutralPatternResults[3] = isSpinningTop
	neutralPatternStrengths[3] = neutralPatternStrengthValues[3]

	neutralPatternResults[4] = isGappingDoji
	neutralPatternStrengths[4] = neutralPatternStrengthValues[4]

	neutralPatternResults[5] = isHaramiCrossBull
	neutralPatternStrengths[5] = neutralPatternStrengthValues[5]

	neutralPatternResults[6] = isHaramiCrossBear
	neutralPatternStrengths[6] = neutralPatternStrengthValues[6]

	neutralPatternResults[7] = isUpsideTasukiGap
	neutralPatternStrengths[7] = neutralPatternStrengthValues[7]

	neutralPatternResults[8] = isOnNeckLine
	neutralPatternStrengths[8] = neutralPatternStrengthValues[8]

	neutralPatternResults[9] = isInNeckLine
	neutralPatternStrengths[9] = neutralPatternStrengthValues[9]

	neutralPatternResults[10] = is3BarInsideBull
	neutralPatternStrengths[10] = neutralPatternStrengthValues[10]

	neutralPatternResults[11] = is3BarInsideBear
	neutralPatternStrengths[11] = neutralPatternStrengthValues[11]

	neutralPatternResults[12] = isHomingPigeon
	neutralPatternStrengths[12] = neutralPatternStrengthValues[12]

	neutralPatternResults[13] = isLastEngulfingBottom
	neutralPatternStrengths[13] = neutralPatternStrengthValues[13]

	neutralPatternResults[14] = isLastEngulfingTop
	neutralPatternStrengths[14] = neutralPatternStrengthValues[14]

	neutralPatternResults[15] = isCounterattackBull
	neutralPatternStrengths[15] = neutralPatternStrengthValues[15]

	neutralPatternResults[16] = isCounterattackBear
	neutralPatternStrengths[16] = neutralPatternStrengthValues[16]

	neutralPatternResults[17] = isThreeStarsSouth
	neutralPatternStrengths[17] = neutralPatternStrengthValues[17]

	neutralPatternResults[18] = isThreeStarsNorth
	neutralPatternStrengths[18] = neutralPatternStrengthValues[18]

	neutralPatternResults[19] = isSqueezeAlert
	neutralPatternStrengths[19] = neutralPatternStrengthValues[19]

	neutralPatternResults[20] = isStalledPattern
	neutralPatternStrengths[20] = neutralPatternStrengthValues[20]

	neutralPatternResults[21] = isUpsideDownsideGap3Bull
	neutralPatternStrengths[21] = neutralPatternStrengthValues[21]

	neutralPatternResults[22] = isUpsideDownsizeGap3Bear
	neutralPatternStrengths[22] = neutralPatternStrengthValues[22]

	neutralPatternResults[23] = isEngulfingDoji
	neutralPatternStrengths[23] = neutralPatternStrengthValues[23]

	neutralPatternResults[24] = isHighWaveCandle
	neutralPatternStrengths[24] = neutralPatternStrengthValues[24]

	neutralPatternResults[25] = isOneBarReversalBull
	neutralPatternStrengths[25] = neutralPatternStrengthValues[25]

	neutralPatternResults[26] = isOneBarReversalBear
	neutralPatternStrengths[26] = neutralPatternStrengthValues[26]

	neutralPatternResults[27] = isThreeGapUp
	neutralPatternStrengths[27] = neutralPatternStrengthValues[27]

	neutralPatternResults[28] = isThreeGapDown
	neutralPatternStrengths[28] = neutralPatternStrengthValues[28]

	neutralPatternResults[29] = isTwoCrows
	neutralPatternStrengths[29] = neutralPatternStrengthValues[29]

	neutralPatternResults[30] = isMorningDojiStar
	neutralPatternStrengths[30] = neutralPatternStrengthValues[30]

	neutralPatternResults[31] = isEveningDojiStar
	neutralPatternStrengths[31] = neutralPatternStrengthValues[31]

	neutralPatternResults[32] = isAdvancingBlock
	neutralPatternStrengths[32] = neutralPatternStrengthValues[32]

	neutralPatternResults[33] = isKickingIndecision
	neutralPatternStrengths[33] = neutralPatternStrengthValues[33]

	// --------------------------------------------------------------
	// 5️⃣  Compute average strengths, most recent pattern & auxiliary filters
	// --------------------------------------------------------------

	// ----- bullish aggregate -----
	bullishCount, bullishStrengthSum := 0, 0.0
	for i := 0; i < len(bullishPatternResults); i++ {
		if bullishPatternResults[i] && bullishPatternStrengths[i] >= minPatternStrength {
			bullishCount++
			bullishStrengthSum += bullishPatternStrengths[i]
		}
	}
	avgBullishStrength := 0.0
	if bullishCount > 0 {
		avgBullishStrength = bullishStrengthSum / float64(bullishCount)
	}

	// ----- bearish aggregate -----
	bearishCount, bearishStrengthSum := 0, 0.0
	for i := 0; i < len(bearishPatternResults); i++ {
		if bearishPatternResults[i] && bearishPatternStrengths[i] >= minPatternStrength {
			bearishCount++
			bearishStrengthSum += bearishPatternStrengths[i]
		}
	}
	avgBearishStrength := 0.0
	if bearishCount > 0 {
		avgBearishStrength = bearishStrengthSum / float64(bearishCount)
	}

	// ----- neutral aggregate -----
	neutralCount, neutralStrengthSum := 0, 0.0
	for i := 0; i < len(neutralPatternResults); i++ {
		if neutralPatternResults[i] && neutralPatternStrengths[i] >= minPatternStrength {
			neutralCount++
			neutralStrengthSum += neutralPatternStrengths[i]
		}
	}
	avgNeutralStrength := 0.0
	if neutralCount > 0 {
		avgNeutralStrength = neutralStrengthSum / float64(neutralCount)
	}

	// Support / resistance pivots (simple approximation – we just look at the most recent pivot)
	// In Pine‑Script `ta.pivothigh(high, swingPivotLength, swingPivotLength)` returns a series
	// that is `na` except on the pivot bar. We emulate it by scanning backwards.
	_, pl := helper.GetHLPivot(highs, lows, swingPivotLength)

	isNearSupport := !math.IsNaN(pl) && closes[i] >= pl*(1-srTolerancePerc) && closes[i] <= pl*(1+srTolerancePerc)
	// isNearResistance := !math.IsNaN(ph) && closes[i] <= ph*(1+srTolerancePerc) && closes[i] >= ph*(1-srTolerancePerc)

	// Follow‑through filter (we use the same rule as the script)
	isFollowThroughBull := helper.IsBullish(opens[i], closes[i])
	// isFollowThroughBear := helper.IsBearish(opens[i], closes[i])

	// Higher‑TF trend filter
	isHTFUp := closes[i] > htfMAVal
	// isHTFDown := closes[i] < htfMAVal

	// --------------------------------------------------------------
	// 6️⃣  Final entry / exit logic (identical to the Pine‑Script)
	// --------------------------------------------------------------
	longSignal := avgBullishStrength >= minAvgStrength &&
		avgBearishStrength < minAvgStrength &&
		isUptrend && isVolumeSpike && isNearSupport && isFollowThroughBull && isHTFUp

	// shortSignal := avgBearishStrength >= minAvgStrength &&
	// 	avgBullishStrength < minAvgStrength &&
	// 	isDowntrend && isVolumeSpike && isNearResistance && isFollowThroughBear && isHTFDown

	exitLong := (avgBearishStrength >= minAvgStrength && isDowntrend) ||
		(avgNeutralStrength >= minAvgStrength)

	// exitShort := (avgBullishStrength >= minAvgStrength && isUptrend) ||
	// 	(avgNeutralStrength >= minAvgStrength)

	if longSignal && !s.State[symbol].InPosition { // || (s.state[symbol].inPosition && exitShort)
		return models.Signal{
			Symbol:  symbol,
			Type:    enum.SignalBuy,
			Percent: avgBullishStrength, // you can map this to any % you want
			Time:    time.Now(),
		}
	}
	if s.State[symbol].InPosition && exitLong { // (shortSignal && !s.state[symbol].inPosition) ||
		return models.Signal{
			Symbol:  symbol,
			Type:    enum.SignalSell,
			Percent: 100,
			Time:    time.Now(),
		}
	}

	return models.Signal{Symbol: symbol, Type: enum.SignalHold, Percent: 0, Time: time.Now()}
}

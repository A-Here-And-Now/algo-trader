package enum

import (
	"fmt"
	"time"
)

type CandleSize int

const (
	CandleSize1m CandleSize = iota
	CandleSize5m
	CandleSize15m
	CandleSize30m
	CandleSize1h
	CandleSize2h
	CandleSize4h
	CandleSize6h
	CandleSize1d
)

func (c CandleSize) String() string {
	switch c {
	case CandleSize1m:
		return "CandleSize1m"
	case CandleSize5m:
		return "CandleSize5m"
	case CandleSize15m:
		return "CandleSize15m"
	case CandleSize30m:
		return "CandleSize30m"
	case CandleSize1h:
		return "CandleSize1h"
	case CandleSize2h:
		return "CandleSize2h"
	case CandleSize4h:
		return "CandleSize4h"
	case CandleSize6h:
		return "CandleSize6h"
	case CandleSize1d:
		return "CandleSize1d"
	default:
		panic(fmt.Sprintf("Unknown CandleSize (%d)", c))
	}
}

func GetCandleSizeFromString(s string) CandleSize {
	switch s {
	case "CandleSize1m":
		return CandleSize1m
	case "CandleSize5m":
		return CandleSize5m
	case "CandleSize15m":
		return CandleSize15m
	case "CandleSize30m":
		return CandleSize30m
	case "CandleSize1h":
		return CandleSize1h
	case "CandleSize2h":
		return CandleSize2h
	case "CandleSize4h":
		return CandleSize4h
	case "CandleSize6h":
		return CandleSize6h
	case "CandleSize1d":
		return CandleSize1d
	default:
		panic(fmt.Sprintf("Unknown CandleSize (%s)", s))
	}
}

func GetCoinbaseGranularityFromCandleSize(candleSize CandleSize) string {
	switch candleSize {
	case CandleSize1m:
		return "ONE_MINUTE"
	case CandleSize5m:
		return "FIVE_MINUTE"
	case CandleSize15m:
		return "FIFTEEN_MINUTE"
	case CandleSize30m:
		return "THIRTY_MINUTE"
	case CandleSize1h:
		return "ONE_HOUR"
	case CandleSize2h:
		return "TWO_HOUR"
	case CandleSize4h:
		return "FOUR_HOUR"
	case CandleSize6h:
		return "SIX_HOUR"
	case CandleSize1d:
		return "ONE_DAY"
	default:
		panic(fmt.Sprintf("Unknown CandleSize (%d)", candleSize))
	}
}

func GetTimeDurationFromCandleSize(tf CandleSize) time.Duration {
	switch tf {
	case CandleSize1m:
		return time.Minute
	case CandleSize5m:
		return 5 * time.Minute
	case CandleSize15m:
		return 15 * time.Minute
	case CandleSize30m:
		return 30 * time.Minute
	case CandleSize1h:
		return time.Hour
	case CandleSize2h:
		return 2 * time.Hour
	case CandleSize4h:
		return 4 * time.Hour
	case CandleSize6h:
		return 6 * time.Hour
	case CandleSize1d:
		return 24 * time.Hour
	default:
		return time.Microsecond
	}
}

func GetLongCandleSizeFromCandleSize(candleSize CandleSize) CandleSize {
	switch candleSize {
	case CandleSize1m:
		return CandleSize15m
	case CandleSize5m:
		return CandleSize30m
	case CandleSize15m:
		return CandleSize1h
	case CandleSize30m:
		return CandleSize2h
	case CandleSize1h:
		return CandleSize4h
	case CandleSize2h:
		return CandleSize6h
	case CandleSize4h:
		return CandleSize1d
	default:
		panic(fmt.Sprintf("Cannot get long candle size from %d", candleSize))
	}
}
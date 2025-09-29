package enum

import "fmt"

type IndicatorType int

const (
	EMA IndicatorType = iota
	RSI
	MACD
	Stochastic
	BollingerBands
	
	ADX
	ATR
	CCI
)

func (s IndicatorType) String() string {
	switch s {
	case EMA:
		return "EMA"
	case RSI:
		return "RSI"
	case MACD:
		return "MACD"
	case Stochastic:
		return "Stochastic"
	case BollingerBands:
		return "BollingerBands"
	case ADX:
		return "ADX"
	case ATR:
		return "ATR"
	case CCI:
		return "CCI"
	default:
		return ""
	}
}

func GetIndicator(s string) IndicatorType {
	switch s {
	case "EMA":
		return EMA
	case "RSI":
		return RSI
	case "MACD":
		return MACD
	case "Stochastic":
		return Stochastic
	case "BollingerBands":
		return BollingerBands
	case "ADX":
		return ADX
	case "ATR":
		return ATR
	case "CCI":
		return CCI
	default:
		panic(fmt.Sprintf("Unknown Indicator (%s)", s))
	}
}

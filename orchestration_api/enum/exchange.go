package enum

import "fmt"

type Exchange int

const (
	ExchangeCoinbase Exchange = iota
	ExchangeUniswap
	ExchangeDeribit
)

func GetExchangeFromString(s string) Exchange {
	switch s {
	case "ExchangeCoinbase":
		return ExchangeCoinbase
	case "ExchangeUniswap":
		return ExchangeUniswap
	case "ExchangeDeribit":
		return ExchangeDeribit
	default:
		panic(fmt.Sprintf("Unknown Exchange (%s)", s))
	}
}

func (e Exchange) String() string {
	switch e {
	case ExchangeCoinbase:
		return "ExchangeCoinbase"
	case ExchangeUniswap:
		return "ExchangeUniswap"
	case ExchangeDeribit:
		return "ExchangeDeribit"
	default:
		panic(fmt.Sprintf("Unknown Exchange (%d)", e))
	}
}
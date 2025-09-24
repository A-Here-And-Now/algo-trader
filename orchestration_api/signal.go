package main

import "time"

type Signal struct {
	Symbol  string
	Type    SignalType
	Percent float64 // 0-100 meaning percent of allocated funds or position per rules
	Time    time.Time
}

// SignalType represents buy/sell; hold is omitted (we don't emit holds)
type SignalType int

const (
	SignalBuy SignalType = iota
	SignalSell
)

func (s SignalType) String() string {
	switch s {
	case SignalBuy:
		return "BUY"
	case SignalSell:
		return "SELL"
	default:
		return "UNKNOWN"
	}
}

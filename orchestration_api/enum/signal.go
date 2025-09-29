package enum

import "fmt"

// SignalType represents buy/sell/hold
type SignalType int

const (
	SignalBuy SignalType = iota
	SignalSell
	SignalHold
)

func (s SignalType) String() string {
	switch s {
	case SignalBuy:
		return "SignalBuy"
	case SignalSell:
		return "SignalSell"
	case SignalHold:
		return "SignalHold"
	default:
		return ""
	}
}

func GetSignalType(s string) SignalType {
	switch s {
	case "SignalBuy":
		return SignalBuy
	case "SignalSell":
		return SignalSell
	case "SignalHold":
		return SignalHold
	default:
		panic(fmt.Sprintf("Unknown SignalType (%s)", s))
	}
}

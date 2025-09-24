package main

import "fmt"

type Strategy int

const (
	MeanReversion Strategy = iota
	TrendFollowing
	Momentum
)

func (s Strategy) String() string {
	switch s {
	case MeanReversion:
		return "MeanReversion"
	case TrendFollowing:
		return "TrendFollowing"
	case Momentum:
		return "Momentum"
	default:
		return ""
	}
}

func GetStrategy(s string) Strategy {
	switch s {
	case "MeanReversion":
		return MeanReversion
	case "TrendFollowing":
		return TrendFollowing
	case "Momentum":
		return Momentum
	default:
		panic(fmt.Sprintf("Unknown Strategy (%s)", s))
	}
}

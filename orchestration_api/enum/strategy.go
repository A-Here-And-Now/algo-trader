package enum

import "fmt"

type Strategy int

// this says its a strategy template, maybe worth looking athttps://www.tradingview.com/script/tRduV2Gy-The-Best-Strategy-Template-LuciTech/
const (
	MeanReversion Strategy = iota	// https://www.tradingview.com/script/zjPWQO39-The-Barking-Rat-Lite/
	TrendFollowing                  // https://www.tradingview.com/script/mVkDf8qh-TrendMaster-Pro-2-3-with-Alerts/
	CandlestickAggregation    		// https://www.tradingview.com/script/ubNhdO2q-Grand-Master-s-Candlestick-Dominance-ATR-Enhanced/
	RenkoCandlesticks               // https://www.tradingview.com/script/O3qQrueT-Triple-Quad-Frosty-v4-5/
	HeikenAshi                      // https://www.tradingview.com/script/EdeSmT9i-Mutanabby-AI-ATR-Trend-Following-Strategy/
	TurtleTrader                    // https://www.tradingview.com/script/4IWFtUWm-Donchian-Fibonacci-Trading-Tool/ also see orchestration_api/TurtleTrading.md
	TrendlineBreakout               // https://www.tradingview.com/script/grMQIRAr-Trendline-Breakout-Strategy-KedArc-Quant/ and https://www.tradingview.com/script/4juJumUH-Instant-Breakout-Strategy-with-RSI-VWAP/
	Supertrend                      // https://www.tradingview.com/script/r6dAP7yi/ and https://www.tradingview.com/script/Y0KEwo8o-script-algo-orb-strategy-with-filters/
	GroverLlorensActivator          // https://www.tradingview.com/script/VuYM89Tw-Grover-Llorens-Activator-Strategy-Analysis/
)

func (s Strategy) String() string {
	switch s {
	case MeanReversion:
		return "MeanReversion"
	case TrendFollowing:
		return "TrendFollowing"
	case CandlestickAggregation:
		return "CandlestickAggregation"
	case RenkoCandlesticks:
		return "RenkoCandlesticks"
	case HeikenAshi:
		return "HeikenAshi"
	case TurtleTrader:
		return "TurtleTrader"
	case TrendlineBreakout:
		return "TrendlineBreakout"
	case Supertrend:
		return "Supertrend"
	case GroverLlorensActivator:
		return "GroverLlorensActivator"
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
	case "CandlestickAggregation":
		return CandlestickAggregation
	case "RenkoCandlesticks":
		return RenkoCandlesticks
	case "HeikenAshi":
		return HeikenAshi
	case "TurtleTrader":
		return TurtleTrader
	case "TrendlineBreakout":
		return TrendlineBreakout
	case "Supertrend":
		return Supertrend
	case "GroverLlorensActivator":
		return GroverLlorensActivator
	default:
		panic(fmt.Sprintf("Unknown Strategy (%s)", s))
	}
}

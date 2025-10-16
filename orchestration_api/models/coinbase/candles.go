package coinbase

type CandlesResponse struct {
	Candles []CoinbaseHistoricalCandle `json:"candles"`
}

type CoinbaseHistoricalCandle struct {
	Start     string  `json:"start"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Open      float64 `json:"open"`
	Close     float64 `json:"close"`
	Volume    float64 `json:"volume"`
}

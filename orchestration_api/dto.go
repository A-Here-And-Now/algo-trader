package main

type tickerMsg struct {
    
	Channel       string  `json:"channel"`      // "ticker"
    ClientID 	  string  `json:"client_id"`
    Timestamp     string  `json:"timestamp"`      // ISO‑8601 timestamp
	SequenceNum   int     `json:"sequence_num"`
	Events        []struct {
		Type       string      `json:"type"`
		Tickers    []Ticker `json:"tickers"`
	}      `json:"events"`
}

type Ticker struct {
	Type string `json:"type"`
	ProductID string `json:"product_id"`
	Price     string `json:"price"`
	Volume24H string `json:"volume_24_h"`
	Low24H    string `json:"low_24_h"`
	High24H   string `json:"high_24_h"`
	Low52W    string `json:"low_52_w"`
	High52W   string `json:"high_52_w"`
	PricePercentChg24H string `json:"price_percent_chg_24_h"`
	BestBid string `json:"best_bid"`
	BestBidQuantity string `json:"best_bid_quantity"`
	BestAsk string `json:"best_ask"`
	BestAskQuantity string `json:"best_ask_quantity"`
}

type candleMsg struct {
	Channel        string      `json:"channel"`
	ProductID 	   string      `json:"product_id"`
	ClientID       string      `json:"client_id"`
	Timestamp      string      `json:"timestamp"`
	SequenceNum    int         `json:"sequence_num"`
	Events         []struct {
		Type       string      `json:"type"`
		Candles    []Candle `json:"candles"`
	}      `json:"events"`
}

type Candle struct {
    Start  string  `json:"start"`
    High   float64 `json:"high"`
    Low    float64 `json:"low"`
    Open   float64 `json:"open"`
    Close  float64 `json:"close"`
    Volume float64 `json:"volume"`
	ProductID string
}
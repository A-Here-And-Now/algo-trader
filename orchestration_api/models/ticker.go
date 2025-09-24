package models

import "time"

type TickerMsg struct {
	Channel     string    `json:"channel"` // "ticker"
	ClientID    string    `json:"client_id"`
	Timestamp   time.Time `json:"timestamp"` // ISO‑8601 timestamp
	SequenceNum int       `json:"sequence_num"`
	Events      []struct {
		Type    string   `json:"type"`
		Tickers []Ticker `json:"tickers"`
	} `json:"events"`
}

type Ticker struct {
	Type               string `json:"type"`
	ProductID          string `json:"product_id"`
	Price              string `json:"price"`
	Volume24H          string `json:"volume_24_h"`
	Low24H             string `json:"low_24_h"`
	High24H            string `json:"high_24_h"`
	Low52W             string `json:"low_52_w"`
	High52W            string `json:"high_52_w"`
	PricePercentChg24H string `json:"price_percent_chg_24_h"`
	BestBid            string `json:"best_bid"`
	BestBidQuantity    string `json:"best_bid_quantity"`
	BestAsk            string `json:"best_ask"`
	BestAskQuantity    string `json:"best_ask_quantity"`
}

type FrontEndTicker struct {
	Type   string    `json:"type"`
	Symbol string    `json:"symbol"`
	Price  string    `json:"price"`
	Time   time.Time `json:"time"`
}

func GetFrontEndTicker(ticker Ticker) FrontEndTicker {
	return FrontEndTicker{
		Type:   ticker.Type,
		Symbol: ticker.ProductID,
		Price:  ticker.Price,
		Time:   time.Now(),
	}
}

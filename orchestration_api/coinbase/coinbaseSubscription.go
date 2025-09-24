package coinbase

type CoinbaseSubscription struct {
	Type       string   `json:"type"`
	ProductIDs []string `json:"product_ids"`
	Channel    string   `json:"channel"`
}

func GetMarketSubscriptionPayload(productIDs []string) []CoinbaseSubscription {
	return []CoinbaseSubscription{
		{
			Type:       "subscribe",
			ProductIDs: productIDs,
			Channel:    "ticker_batch",
		},
		{
			Type:       "subscribe",
			ProductIDs: productIDs,
			Channel:    "candles",
		},
	}
}

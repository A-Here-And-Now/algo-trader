package coinbase

type CoinbaseSubscription struct {
	Type       string   `json:"type"`
	ProductIDs []string `json:"product_ids"`
	Channel    string   `json:"channel"`
}

func GetMarketSubscriptionPayload(productIDs []string, isUnsubscribe bool) []CoinbaseSubscription {
	subType := "subscribe"
	if isUnsubscribe {
		subType = "unsubscribe"
	}
	return []CoinbaseSubscription{
		{
			Type:       subType,
			ProductIDs: productIDs,
			Channel:    "candles",
		},
	}
}

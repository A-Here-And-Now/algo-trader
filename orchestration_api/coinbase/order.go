package coinbase

type Order struct {
	ProductID          string `json:"product_id"`
	OrderID            string `json:"order_id"`
	ClientOrderID      string `json:"client_order_id"`
	OrderSide          string `json:"order_side"`
	OrderType          string `json:"order_type"`
	Status             string `json:"status"`
	CompletionPct      string `json:"completion_percentage"`
	CumulativeQuantity string `json:"cumulative_quantity"`
	FilledValue        string `json:"filled_value"`
	Leaves             string `json:"leaves_quantity"`
	LimitPrice         string `json:"limit_price"`
	AvgPrice           string `json:"avg_price"`
	CreationTime       string `json:"creation_time"`
}
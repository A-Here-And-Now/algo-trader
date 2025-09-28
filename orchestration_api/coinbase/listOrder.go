package coinbase

type ListOrder struct {
	OrderID            string `json:"order_id"`
	ProductID          string `json:"product_id"`
	OrderType          string `json:"order_type"`
	OrderSide          string `json:"order_side"`
	Status             string `json:"status"`
	ClientOrderID      string `json:"client_order_id"`
	CreatedTime        string `json:"created_time"`
	CompletionTime     string `json:"completion_time"`
	Price              string `json:"price"`
	AverageFilledPrice string `json:"average_filled_price"`
	FilledSize         string `json:"filled_size"`
	RemainingSize      string `json:"remaining_size"`
}

type ListOrdersResponse struct {
	Orders  []ListOrder `json:"orders"`
	HasNext bool        `json:"has_next"`
	Cursor  string      `json:"cursor"`
}
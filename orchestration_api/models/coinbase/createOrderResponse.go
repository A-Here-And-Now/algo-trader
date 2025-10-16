package coinbase

type CreateOrderResponse struct {
	Success bool   `json:"success"`
	OrderID string `json:"order_id"`
	Error   string `json:"error_message"`
}
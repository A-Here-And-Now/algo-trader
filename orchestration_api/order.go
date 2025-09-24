package main

import "time"

type OrderUpdate struct {
	Channel     string    `json:"channel"` // e.g., "orders"
	ProductID   string    `json:"product_id"`
	OrderID     string    `json:"order_id"`
	Status      string    `json:"status"` // open, filled, cancelled, etc.
	FilledQty   string    `json:"filled_qty"`
	FilledValue string    `json:"filled_value"`
	CompletionPct string    `json:"completion_pct"`
	Leaves    	string    `json:"leaves"`
	Price     	string    `json:"price"`
	Side      	string    `json:"side"`
	Ts        	time.Time `json:"ts"`
}

func (o Order) toOrderUpdate() OrderUpdate {
	return OrderUpdate{
		Channel:     "orders",
		ProductID:   o.ProductID,
		OrderID:     o.OrderID,
		Status:    	 o.Status,
		FilledQty:   o.CumulativeQuantity,
		FilledValue: o.FilledValue,
		CompletionPct: o.CompletionPct,
		Leaves:    	 o.Leaves,
		Price:     	 o.AvgPrice,
		Side:      	 o.OrderSide,
		Ts:        	 time.Now(),
	}
}

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
package models

import (
	"time"
)

type OrderUpdate struct {
	Channel       string    `json:"channel"`
	ProductID     string    `json:"product_id"`
	OrderID       string    `json:"order_id"`
	Status        string    `json:"status"`
	FilledQty     string    `json:"filled_qty"`
	FilledValue   string    `json:"filled_value"`
	CompletionPct string    `json:"completion_pct"`
	Leaves    	  string    `json:"leaves"`
	Price     	  string    `json:"price"`
	Side      	  string    `json:"side"`
	Ts        	  time.Time `json:"ts"`
}
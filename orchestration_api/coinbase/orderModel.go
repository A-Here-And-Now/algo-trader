package coinbase

import (
	"fmt"

	"github.com/google/uuid"
)

// Order configuration types based on Coinbase Advanced Trade API
type MarketMarketIOC struct {
	QuoteSize string `json:"quote_size,omitempty"`
	BaseSize  string `json:"base_size,omitempty"`
}

type MarketMarketFOK struct {
	QuoteSize string `json:"quote_size,omitempty"`
	BaseSize  string `json:"base_size,omitempty"`
}

type SORLimitIOC struct {
	QuoteSize  string `json:"quote_size,omitempty"`
	BaseSize   string `json:"base_size,omitempty"`
	LimitPrice string `json:"limit_price"`
}

type LimitLimitGTC struct {
	QuoteSize  string `json:"quote_size,omitempty"`
	BaseSize   string `json:"base_size,omitempty"`
	LimitPrice string `json:"limit_price"`
	PostOnly   bool   `json:"post_only"`
}

type LimitLimitGTD struct {
	QuoteSize  string `json:"quote_size,omitempty"`
	BaseSize   string `json:"base_size,omitempty"`
	LimitPrice string `json:"limit_price"`
	EndTime    string `json:"end_time"`
	PostOnly   bool   `json:"post_only"`
}

type LimitLimitFOK struct {
	QuoteSize  string `json:"quote_size,omitempty"`
	BaseSize   string `json:"base_size,omitempty"`
	LimitPrice string `json:"limit_price"`
}

type TWAPLimitGTD struct {
	QuoteSize      string `json:"quote_size,omitempty"`
	BaseSize       string `json:"base_size,omitempty"`
	StartTime      string `json:"start_time"`
	EndTime        string `json:"end_time"`
	LimitPrice     string `json:"limit_price"`
	NumberBuckets  string `json:"number_buckets"`
	BucketSize     string `json:"bucket_size"`
	BucketDuration string `json:"bucket_duration"`
}

type StopLimitStopLimitGTC struct {
	BaseSize      string `json:"base_size"`
	LimitPrice    string `json:"limit_price"`
	StopPrice     string `json:"stop_price"`
	StopDirection string `json:"stop_direction"`
}

type StopLimitStopLimitGTD struct {
	BaseSize      string `json:"base_size"`
	LimitPrice    string `json:"limit_price"`
	StopPrice     string `json:"stop_price"`
	EndTime       string `json:"end_time"`
	StopDirection string `json:"stop_direction"`
}

type TriggerBracketGTC struct {
	BaseSize         string `json:"base_size"`
	LimitPrice       string `json:"limit_price"`
	StopTriggerPrice string `json:"stop_trigger_price"`
}

type TriggerBracketGTD struct {
	BaseSize         string `json:"base_size"`
	LimitPrice       string `json:"limit_price"`
	StopTriggerPrice string `json:"stop_trigger_price"`
	EndTime          string `json:"end_time"`
}

type OrderConfiguration struct {
	MarketMarketIOC       *MarketMarketIOC       `json:"market_market_ioc,omitempty"`
	MarketMarketFOK       *MarketMarketFOK       `json:"market_market_fok,omitempty"`
	SORLimitIOC           *SORLimitIOC           `json:"sor_limit_ioc,omitempty"`
	LimitLimitGTC         *LimitLimitGTC         `json:"limit_limit_gtc,omitempty"`
	LimitLimitGTD         *LimitLimitGTD         `json:"limit_limit_gtd,omitempty"`
	LimitLimitFOK         *LimitLimitFOK         `json:"limit_limit_fok,omitempty"`
	TWAPLimitGTD          *TWAPLimitGTD          `json:"twap_limit_gtd,omitempty"`
	StopLimitStopLimitGTC *StopLimitStopLimitGTC `json:"stop_limit_stop_limit_gtc,omitempty"`
	StopLimitStopLimitGTD *StopLimitStopLimitGTD `json:"stop_limit_stop_limit_gtd,omitempty"`
	TriggerBracketGTC     *TriggerBracketGTC     `json:"trigger_bracket_gtc,omitempty"`
	TriggerBracketGTD     *TriggerBracketGTD     `json:"trigger_bracket_gtd,omitempty"`
}

type CreateOrderRequest struct {
	ClientOrderID              string              `json:"client_order_id"`
	ProductID                  string              `json:"product_id"`
	Side                       string              `json:"side"` // "BUY" or "SELL"
	OrderConfiguration         OrderConfiguration  `json:"order_configuration"`
	Leverage                   string              `json:"leverage,omitempty"`
	MarginType                 string              `json:"margin_type,omitempty"` // "CROSS" or "ISOLATED"
	RetailPortfolioID          string              `json:"retail_portfolio_id,omitempty"`
	PreviewID                  string              `json:"preview_id,omitempty"`
	AttachedOrderConfiguration *OrderConfiguration `json:"attached_order_configuration,omitempty"`
	SORPreference              string              `json:"sor_preference,omitempty"`
}

func GetOrderRequest(symbol string, amount float64, isBuy bool, isSellTokensRequest bool) CreateOrderRequest {
	side := "SELL"
	if isBuy {
		side = "BUY"
	}
	orderReq := CreateOrderRequest{
		ClientOrderID: uuid.New().String(),
		ProductID: symbol,
		Side: side,
		OrderConfiguration: OrderConfiguration{
			MarketMarketIOC: &MarketMarketIOC{ },
		},
	}
	if isSellTokensRequest {
		orderReq.OrderConfiguration.MarketMarketIOC.BaseSize = fmt.Sprintf("%f", amount)
	} else {
		orderReq.OrderConfiguration.MarketMarketIOC.QuoteSize = fmt.Sprintf("%f", amount)
	}
	return orderReq
}

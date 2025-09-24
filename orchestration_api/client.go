package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type CoinbaseClient struct {
	baseURL string
	http    *http.Client
}

func NewCoinbaseClient(baseURL string) *CoinbaseClient {
	return &CoinbaseClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// do autogenerates a fresh JWT per request and sets Authorization: Bearer <jwt>.
func (c *CoinbaseClient) do(ctx context.Context, req *http.Request, v any) error {
	jwtTok, err := buildJWT(apiKey, apiSecret)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+jwtTok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("coinbase http %d", resp.StatusCode)
	}
	if v != nil {
		return json.NewDecoder(resp.Body).Decode(v)
	}
	return nil
}

// ===== Typed models based on Coinbase docs =====

type Money struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

type Account struct {
	UUID              string `json:"uuid"`
	Name              string `json:"name"`
	Currency          string `json:"currency"`
	AvailableBalance  Money  `json:"available_balance"`
	Default           bool   `json:"default"`
	Active            bool   `json:"active"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
	DeletedAt         string `json:"deleted_at"`
	Type              string `json:"type"`
	Ready             bool   `json:"ready"`
	Hold              Money  `json:"hold"`
	RetailPortfolioID string `json:"retail_portfolio_id"`
	Platform          string `json:"platform"`
}

type AccountsListResponse struct {
	Accounts []Account `json:"accounts"`
	HasNext  bool      `json:"has_next"`
	Cursor   string    `json:"cursor"`
	Size     int       `json:"size"`
}

type AccountResponse struct {
	Account Account `json:"account"`
}

// ListAccounts returns authenticated accounts
func (c *CoinbaseClient) ListAccounts(ctx context.Context, limit int, cursor string) (AccountsListResponse, error) {
	url := fmt.Sprintf("%s/api/v3/brokerage/accounts", c.baseURL)
	if limit > 0 && cursor != "" {
		url = fmt.Sprintf("%s?limit=%d&cursor=%s", url, limit, cursor)
	}
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	var out AccountsListResponse
	return out, c.do(ctx, req, &out)
}

// ListOrders fetches recent orders
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

func (c *CoinbaseClient) ListOrders(ctx context.Context, productID string, limit int) (ListOrdersResponse, error) {
	url := fmt.Sprintf("%s/api/v3/brokerage/orders/historical/batch?product_id=%s&limit=%d", c.baseURL, productID, limit)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	var out ListOrdersResponse
	return out, c.do(ctx, req, &out)
}

// CreateOrder creates a new order
type CreateOrderResponse struct {
	Success bool   `json:"success"`
	OrderID string `json:"order_id"`
	Error   string `json:"error_message"`
}

func (c *CoinbaseClient) CreateOrder(ctx context.Context, productID string, amountOfUSD float64, isBuy bool) (CreateOrderResponse, error) {
	body := GetOrderRequest(productID, amountOfUSD, isBuy)
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return CreateOrderResponse{}, err
	}
	url := fmt.Sprintf("%s/api/v3/brokerage/orders", c.baseURL)
	req, _ := http.NewRequest(http.MethodPost, url, bytesReader(jsonBody))
	var out CreateOrderResponse
	return out, c.do(ctx, req, &out)
}

// EditOrder edits an existing order
type EditOrderResponse struct {
	Success bool   `json:"success"`
	OrderID string `json:"order_id"`
	Error   string `json:"error_message"`
}

func (c *CoinbaseClient) EditOrder(ctx context.Context, body []byte) (EditOrderResponse, error) {
	url := fmt.Sprintf("%s/api/v3/brokerage/orders/edit", c.baseURL)
	req, _ := http.NewRequest(http.MethodPost, url, bytesReader(body))
	var out EditOrderResponse
	return out, c.do(ctx, req, &out)
}

// CancelOrder cancels an order
func (c *CoinbaseClient) CancelOrder(ctx context.Context, orderID string) error {
	url := fmt.Sprintf("%s/api/v3/brokerage/orders/%s", c.baseURL, orderID)
	req, _ := http.NewRequest(http.MethodDelete, url, nil)
	return c.do(ctx, req, nil)
}

// ClosePosition closes current position for a product
type ClosePositionResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error_message"`
}

type ClosePositionRequest struct {
	ProductID string `json:"product_id"`
}

func (c *CoinbaseClient) ClosePosition(ctx context.Context, closeReq ClosePositionRequest) (ClosePositionResponse, error) {
	body, err := json.Marshal(closeReq)
	if err != nil {
		return ClosePositionResponse{}, err
	}
	url := fmt.Sprintf("%s/api/v3/brokerage/orders/close-position", c.baseURL)
	req, _ := http.NewRequest(http.MethodPost, url, bytesReader(body))
	var out ClosePositionResponse
	return out, c.do(ctx, req, &out)
}

// helper to avoid importing bytes directly elsewhere
func bytesReader(b []byte) *bytesReaderWrapper { return &bytesReaderWrapper{b: b} }

type bytesReaderWrapper struct{ b []byte }

func (w *bytesReaderWrapper) Read(p []byte) (int, error) {
	if len(w.b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, w.b)
	w.b = w.b[n:]
	return n, nil
}

func (w *bytesReaderWrapper) Close() error { return nil }

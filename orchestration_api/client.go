package main

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type CoinbaseClient struct {
	baseURL string
	http    *http.Client
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

func NewCoinbaseClient(baseURL string) *CoinbaseClient {
	return &CoinbaseClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func buildJWT(apiKey, apiSecret string) (string, error) {
	// Typical claims: iat (issued at), exp (expiry), sub (subject) or apikey
	apiKey = os.Getenv("COINBASE_API_KEY")

	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"iat": now.Unix(),
		"exp": now.Add(2 * time.Minute).Unix(), // short-lived token
		"sub": apiKey,                          // example — use actual claim names required by provider
		// add other claims required by the API (e.g., "kid", "scope", "aud", etc.)
	}

	// Coinbase advanced trade user websocket expects ES256 with your API private key.
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	block, _ := pem.Decode([]byte(os.Getenv("COINBASE_PRIVATE_KEY_PEM")))
	if block == nil {
		log.Fatal("failed to decode PEM block")
	}

	privKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		log.Fatalf("failed to parse EC private key: %v", err)
	}

	// If the API requires a keyed HMAC signature but with a hex key, you might compute it differently.
	// The simple case: use the API secret bytes as the HMAC key.
	return token.SignedString(privKey)
}

// sendWithJwt autogenerates a fresh JWT per request and sets Authorization: Bearer <jwt>.
func (c *CoinbaseClient) sendWithJwt(ctx context.Context, req *http.Request, v any) error {
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

func (c *CoinbaseClient) ListAccounts(ctx context.Context) (AccountsListResponse, error) {
	url := fmt.Sprintf("%s/api/v3/brokerage/accounts", c.baseURL)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	var out AccountsListResponse
	return out, c.sendWithJwt(ctx, req, &out)
}

func (c *CoinbaseClient) GetAllTokenBalances(ctx context.Context) (map[string]float64, error) {
	var balances map[string]float64 = make(map[string]float64)

	response, err := c.ListAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}

	// Extract token balances from accounts
	for _, account := range response.Accounts {
		// Only include accounts that are active and ready
		if account.Active && account.Ready {
			// Calculate total balance (available + hold)
			availableVal := account.AvailableBalance.Value
			balances[account.Currency] = parseFloatSafe(availableVal)
		}
	}

	return balances, nil
}

// Helper function to safely parse float strings
func parseFloatSafe(s string) float64 {
	if s == "" {
		return 0
	}
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return val
}

func (c *CoinbaseClient) ListOrders(ctx context.Context, productID string, limit int) (ListOrdersResponse, error) {
	url := fmt.Sprintf("%s/api/v3/brokerage/orders/historical/batch?product_id=%s&limit=%d", c.baseURL, productID, limit)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	var out ListOrdersResponse
	return out, c.sendWithJwt(ctx, req, &out)
}

// CreateOrder creates a new order
type CreateOrderResponse struct {
	Success bool   `json:"success"`
	OrderID string `json:"order_id"`
	Error   string `json:"error_message"`
}

func (c *CoinbaseClient) CreateOrder(ctx context.Context, productID string, amountOfUSD float64, isBuy bool) (CreateOrderResponse, error) {
	body := GetOrderRequest(productID, amountOfUSD, isBuy, false)
	return c.privateCreateOrder(ctx, body)
}

func (c *CoinbaseClient) SellTokens(productID string, amountOfUSD float64) (CreateOrderResponse, error) {
	body := GetOrderRequest(productID, amountOfUSD, false, true)
	return c.privateCreateOrder(context.Background(), body)
}

func (c *CoinbaseClient) privateCreateOrder(ctx context.Context, body CreateOrderRequest) (CreateOrderResponse, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return CreateOrderResponse{}, err
	}
	url := fmt.Sprintf("%s/api/v3/brokerage/orders", c.baseURL)
	req, _ := http.NewRequest(http.MethodPost, url, bytesReader(jsonBody))
	var out CreateOrderResponse
	return out, c.sendWithJwt(context.Background(), req, &out)
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
	return out, c.sendWithJwt(ctx, req, &out)
}

// CancelOrder cancels an order
func (c *CoinbaseClient) CancelOrders(ctx context.Context, orderID string) error {
	url := fmt.Sprintf("%s/api/v3/brokerage/orders/batch_cancel", c.baseURL)
	// marshall orderID into a single element array
	body, err := json.Marshal([]string{orderID})
	if err != nil {
		return err
	}
	req, _ := http.NewRequest(http.MethodDelete, url, bytesReader(body))
	return c.sendWithJwt(ctx, req, nil)
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

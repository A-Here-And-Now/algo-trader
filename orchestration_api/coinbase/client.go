package coinbase

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	"github.com/golang-jwt/jwt/v5"
)

type CoinbaseClient struct {
	baseURL   string
	http      *http.Client
	apiKey    string
	apiSecret string
}

// ===== Typed models based on Coinbase docs =====

type Money struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

func NewCoinbaseClient(baseURL string, apiKey string, apiSecret string) *CoinbaseClient {
	return &CoinbaseClient{
		baseURL:   baseURL,
		http:      &http.Client{Timeout: 10 * time.Second},
		apiKey:    apiKey,
		apiSecret: apiSecret,
	}
}

func BuildJWT(apiKey, apiSecret string) (string, error) {

	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"iat": now.Unix(),
		"exp": now.Add(2 * time.Minute).Unix(), // short-lived token
		"sub": apiKey,                          // example — use actual claim names required by provider
		// add other claims required by the API (e.g., "kid", "scope", "aud", etc.)
	}

	// Coinbase advanced trade user websocket expects ES256 with your API private key.
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	block, _ := pem.Decode([]byte(apiSecret))
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
	jwtTok, err := BuildJWT(c.apiKey, c.apiSecret)
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

func (c *CoinbaseClient) send(ctx context.Context, req *http.Request, v any) error {
	req = req.WithContext(ctx)
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

func (c *CoinbaseClient) GetHistoricalCandles(ctx context.Context, productID string, candleSize enum.CandleSize) (CandlesResponse, error) {
	url := fmt.Sprintf("%s/api/v3/brokerage/market/products/%s/candles", c.baseURL, productID)
	req, _ := http.NewRequest(http.MethodGet, url, nil)

	startUnix := time.Now().Add(-100 * enum.GetTimeDurationFromCandleSize(candleSize)).Unix() // 26-ish days ago (should be 314 candles aka buckets)
	endUnix := time.Now().Unix() 

	// Convert the int64 values to strings for the URL query.
	q := req.URL.Query()
	q.Set("start", strconv.FormatInt(startUnix, 10))
	q.Set("end", strconv.FormatInt(endUnix, 10))
	q.Set("granularity", enum.GetCoinbaseGranularityFromCandleSize(candleSize))
	req.URL.RawQuery = q.Encode()
	var out CandlesResponse
	return out, c.send(ctx, req, &out)
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
			balances[account.Currency] = ParseFloatSafe(availableVal)
		}
	}

	return balances, nil
}

// Helper function to safely parse float strings
func ParseFloatSafe(s string) float64 {
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
	req, _ := http.NewRequest(http.MethodPost, url, BytesReader(jsonBody))
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
	req, _ := http.NewRequest(http.MethodPost, url, BytesReader(body))
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
	req, _ := http.NewRequest(http.MethodDelete, url, BytesReader(body))
	return c.sendWithJwt(ctx, req, nil)
}

// helper to avoid importing bytes directly elsewhere
func BytesReader(b []byte) *bytesReaderWrapper { return &bytesReaderWrapper{b: b} }

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

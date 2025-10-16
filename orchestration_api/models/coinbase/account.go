package coinbase

type TokenHolding struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

type Account struct {
	UUID              string   		  `json:"uuid"`
	Name              string          `json:"name"`
	Symbol            string          `json:"symbol"`
	Currency          string   		  `json:"currency"`
	AvailableBalance  TokenHolding    `json:"available_balance"`
	Default           bool     		  `json:"default"`
	Active            bool     		  `json:"active"`
	CreatedAt         string   		  `json:"created_at"`
	UpdatedAt         string   		  `json:"updated_at"`
	DeletedAt         string   		  `json:"deleted_at"`
	Type              string   		  `json:"type"`
	Ready             bool     		  `json:"ready"`
	Hold              TokenHolding    `json:"hold"`
	RetailPortfolioID string   		  `json:"retail_portfolio_id"`
	Platform          string   		  `json:"platform"`
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
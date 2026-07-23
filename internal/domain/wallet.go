package domain

import "time"

type Currency string

const (
	CurrencyINR Currency = "INR"
	CurrencyUSD Currency = "USD"
	CurrencyMYR Currency = "MYR"
)

type Wallet struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Currency  Currency  `json:"currency"`
	Balance   int64     `json:"balance"` // Store in smallest unit (paise/cents)
	Frozen    bool      `json:"frozen"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

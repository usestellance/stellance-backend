package wallet

import "time"

type WalletResponseDto struct {
	ID            string               `json:"id"`
	UserID        string               `json:"user_id"`
	WalletAddress string               `json:"wallet_address"`
	PrivateKey    string               `json:"private_key,omitempty"`
	Tag           string               `json:"tag,omitempty"`
	Chain         string               `json:"chain,omitempty"`
	Balance       StellarWalletBalance `json:"balance,omitempty"`
	IsPrimary     bool                 `json:"is_primary,omitempty"`
	IsActive      bool                 `json:"is_active,omitempty"`
	CreatedAt     *time.Time           `json:"created_at,omitempty"`
}

type StellarWalletBalance struct {
	USDC float64 `json:"usdc,omitempty"`
	XLM  float64 `json:"xlm,omitempty"`
}

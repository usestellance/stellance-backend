package wallet

import "time"

type SetPinDTO struct {
	Pin string `json:"pin" validate:"required,min=4,max=8,numeric" example:"4821"` // 4-8 digits
}

type PayInvoiceDTO struct {
	InvoiceID   string `json:"invoice_id" validate:"required,uuid" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"` // Invoice to pay
	SourceAsset string `json:"source_asset" validate:"required,oneof=XLM USDC" example:"XLM"`                      // Asset spent from this wallet: XLM or USDC
	Pin         string `json:"pin" validate:"required" example:"4821"`                                             // Wallet PIN
}

type TransferDTO struct {
	DestinationAddress string `json:"destination_address" validate:"required" example:"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"` // Stellar account id (G...)
	Amount             string `json:"amount" validate:"required" example:"10.5"`                                                                  // Amount the recipient receives, in dest_asset
	SourceAsset        string `json:"source_asset" validate:"required,oneof=XLM USDC" example:"XLM"`                                              // Asset spent: XLM or USDC
	DestAsset          string `json:"dest_asset" validate:"required,oneof=XLM USDC" example:"USDC"`                                               // Asset received: XLM or USDC
	Pin                string `json:"pin" validate:"required" example:"4821"`                                                                     // Wallet PIN
}

type PathPaymentResult struct {
	TransactionHash string  `json:"transaction_hash" example:"a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7"` // Stellar transaction hash
	SourceAsset     string  `json:"source_asset" example:"XLM"`                                                                  // Asset spent
	SourceAmount    string  `json:"source_amount" example:"12.3456789"`                                                          // Amount spent, 7 decimals
	DestAsset       string  `json:"dest_asset" example:"USDC"`                                                                   // Asset delivered
	DestAmount      string  `json:"dest_amount" example:"10.5"`                                                                  // Amount delivered
	Destination     string  `json:"destination" example:"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"`              // Recipient account
	Fee             float64 `json:"fee_xlm" example:"0.00001"`                                                                   // Network fee in XLM
	PaymentStatus   string  `json:"payment_status,omitempty" example:"detected"`                                                 // Invoice payments only: detected until the chain data is verified, then verified, invalid or failed
}

type WalletResponseDto struct {
	ID            string                `json:"id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"`                                 // Wallet id
	UserID        string                `json:"user_id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"`                            // Owner
	WalletAddress string                `json:"wallet_address" example:"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"` // Stellar account id
	PrivateKey    string                `json:"private_key,omitempty"`                                                             // Only returned once, at creation
	Tag           string                `json:"tag,omitempty" example:"Main"`                                                      // Label
	Chain         string                `json:"chain,omitempty" example:"stellar"`                                                 // Chain
	Balance       *StellarWalletBalance `json:"balance,omitempty"`
	IsPrimary     bool                  `json:"is_primary,omitempty" example:"true"`                 // Receives invoice payments
	IsActive      bool                  `json:"is_active,omitempty" example:"true"`                  // Active flag
	CreatedAt     *time.Time            `json:"created_at,omitempty" example:"2026-10-20T09:00:00Z"` // Created
}

type StellarWalletBalance struct {
	USDC float64 `json:"usdc,omitempty" example:"5.03"` // USDC balance
	XLM  float64 `json:"xlm,omitempty" example:"10002"` // XLM balance
}

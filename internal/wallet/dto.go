package wallet

import "time"

type SetPinDTO struct {
	Pin string `json:"pin" validate:"required,min=4,max=8,numeric"`
}

type PayInvoiceDTO struct {
	InvoiceID   string `json:"invoice_id" validate:"required,uuid"`
	SourceAsset string `json:"source_asset" validate:"required,oneof=XLM USDC"`
	Pin         string `json:"pin" validate:"required"`
}

type TransferDTO struct {
	DestinationAddress string `json:"destination_address" validate:"required"`
	Amount             string `json:"amount" validate:"required"`
	SourceAsset        string `json:"source_asset" validate:"required,oneof=XLM USDC"`
	DestAsset          string `json:"dest_asset" validate:"required,oneof=XLM USDC"`
	Pin                string `json:"pin" validate:"required"`
}

type PathPaymentResult struct {
	TransactionHash string  `json:"transaction_hash"`
	SourceAsset     string  `json:"source_asset"`
	SourceAmount    string  `json:"source_amount"`
	DestAsset       string  `json:"dest_asset"`
	DestAmount      string  `json:"dest_amount"`
	Destination     string  `json:"destination"`
	Fee             float64 `json:"fee_xlm"`
}

type WalletResponseDto struct {
	ID            string               `json:"id"`
	UserID        string               `json:"user_id"`
	WalletAddress string               `json:"wallet_address"`
	PrivateKey    string               `json:"private_key,omitempty"`
	Tag           string               `json:"tag,omitempty"`
	Chain         string               `json:"chain,omitempty"`
	Balance       *StellarWalletBalance `json:"balance,omitempty"`
	IsPrimary     bool                 `json:"is_primary,omitempty"`
	IsActive      bool                 `json:"is_active,omitempty"`
	CreatedAt     *time.Time           `json:"created_at,omitempty"`
}

type StellarWalletBalance struct {
	USDC float64 `json:"usdc,omitempty"`
	XLM  float64 `json:"xlm,omitempty"`
}

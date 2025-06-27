package transactions

import "time"

type TransactionType string

const (
	WITHDRAWAL TransactionType = "withdrawal"
	FUNDING    TransactionType = "funding"
	PAYMENT    TransactionType = "payment"
)

type TransactionDto struct {
	InvoiceID         *string         `json:"invoice_id,omitempty"`
	WalletID          *string         `json:"wallet_id"`
	TransactionHash   string          `json:"transaction_hash"`
	Amount            float64         `json:"amount"`
	Token             string          `json:"token"`
	TransactionStatus string          `json:"transaction_status"`
	NetworkFee        *float64        `json:"network_fee"`
	ConfirmedAt       *time.Time      `json:"confirmed_at,omitempty"`
	TransactionType   TransactionType `json:"transaction_type"`
	CreatedAt         *time.Time      `json:"created_at"`
}

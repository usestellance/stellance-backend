package transactions

import (
	"time"

	"github.com/gofrs/uuid"
)

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

type GetTransactionDto struct {
	ID              uuid.UUID  `json:"id"`
	InvoiceID       *uuid.UUID `json:"invoice_id,omitempty"`
	WalletID        *uuid.UUID `json:"wallet_id,omitempty"`
	TransactionHash string     `json:"transaction_hash"`
	Amount          float64    `json:"amount"`
	Currency        string     `json:"currency"`
	Status          string     `json:"status"`
	NetworkFee      *float64   `json:"network_fee,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	ConfirmedAt     *time.Time `json:"confirmed_at,omitempty"`
	TokenType       string     `json:"token_type"`
	TransactionType string     `json:"transaction_type"`
}

type PaginatedTransactions struct {
	Data       []GetTransactionDto `json:"data"`
	Page       int                 `json:"page"`
	Limit      int                 `json:"limit"`
	TotalCount int                 `json:"total_count"`
}

type TransactionFiltersDto struct {
	UserId string `json:"user_id,omitempty"`
	Page   int    `json:"page" validate:"required,min=1"`
	Count  int    `json:"count" validate:"required,min=1,max=15"`
}

type TransactionCard struct {
	Amount       int64 `json:"amount"`
	InvoiceCount int   `json:"invoice_count"`
}

type GetTransactionOverViewResponse struct {
	TotalAmount   TransactionCard `json:"total_amount"`
	PendingAmount TransactionCard `json:"pending_amount"`
	PaidAmount    TransactionCard `json:"paid_amount"`
	OverdueAmount TransactionCard `json:"overdue_amount"`
}

type TransactionOverviewRow struct {
	InvoiceStatus string
	TotalAmount string
	InvoiceCount int
}

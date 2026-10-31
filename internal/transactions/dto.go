package transactions

import (
	"time"

	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
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
	ID              uuid.UUID  `json:"id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"`                                           // Ledger entry id
	InvoiceID       *uuid.UUID `json:"invoice_id,omitempty" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"`                         // Related invoice; null for transfers
	WalletID        *uuid.UUID `json:"wallet_id,omitempty" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"`                          // Wallet involved
	TransactionHash string     `json:"transaction_hash" example:"a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7"` // Stellar transaction hash
	Amount          float64    `json:"amount" example:"2.03"`                                                                       // Amount in the entry currency
	Currency        string     `json:"currency" example:"usdc"`                                                                     // usdc or xlm
	Status          string     `json:"status" example:"confirmed"`                                                                  // On-chain result: pending, confirmed, failed, aborted or refunded
	NetworkFee      *float64   `json:"network_fee,omitempty" example:"0.00001"`                                                     // Fee in XLM
	CreatedAt       time.Time  `json:"created_at" example:"2026-10-22T14:03:26Z"`                                                   // Created
	ConfirmedAt     *time.Time `json:"confirmed_at,omitempty" example:"2026-10-22T14:03:11Z"`                                       // Ledger close time
	TokenType       string     `json:"token_type" example:"usdc"`                                                                   // Token
	TransactionType string     `json:"transaction_type" example:"payment"`                                                          // payment, withdrawal, funding or path_payment
	SourceAsset     *string    `json:"source_asset,omitempty" example:"XLM"`                                                        // Asset spent for path payments
	SourceAmount    *string    `json:"source_amount,omitempty" example:"12.3456789"`                                                // Amount spent for path payments
}

type PaginatedTransactions struct {
	Items      []GetTransactionDto `json:"items"`
	Meta       utils.PageMeta      `json:"meta"`
	Data       []GetTransactionDto `json:"data"`                     // deprecated: use items
	Page       int                 `json:"page" example:"1"`         // Deprecated: use meta.page
	Limit      int                 `json:"limit" example:"10"`       // Deprecated: use meta.limit
	TotalCount int                 `json:"total_count" example:"57"` // Deprecated: use meta.total
}

type TransactionFiltersDto struct {
	UserId   string `json:"user_id,omitempty"`
	Page     int    `json:"page" validate:"required,min=1"`
	Count    int    `json:"count" validate:"required,min=1,max=15"`
	WalletID string `json:"wallet_id,omitempty"`
	Type     string `json:"type,omitempty"`
	Status   string `json:"status,omitempty"`
	From     string `json:"from,omitempty"`
	To       string `json:"to,omitempty"`
}

type TransactionCard struct {
	Amount       int64 `json:"amount" example:"45000"`    // Integer cents
	InvoiceCount int   `json:"invoice_count" example:"3"` // Invoices in this bucket
}

type GetTransactionOverViewResponse struct {
	TotalAmount          TransactionCard `json:"total_amount"`
	PendingAmount        TransactionCard `json:"pending_amount"`
	PaidAmount           TransactionCard `json:"paid_amount"`
	OverdueAmount        TransactionCard `json:"overdue_amount"`
	ExpiredAmount        TransactionCard `json:"expired_amount"`
	InvalidPaymentAmount TransactionCard `json:"invalid_payment_amount"`
}

type TransactionOverviewRow struct {
	InvoiceStatus string
	TotalAmount   string
	InvoiceCount  int
}
type TransactionCashFlowQuery struct {
	From string `form:"from" json:"from"` // Format: YYYY-MM-DD
	To   string `form:"to" json:"to"`     // Format: YYYY-MM-DD
}

type CashFlowDataPoint struct {
	Date     string `json:"date" example:"2026-10-01"` // Month start, YYYY-MM-DD
	Amount   int64  `json:"amount" example:"45000"`    // Integer cents
	Invoices int    `json:"invoices" example:"3"`      // Paid invoices
}

type GetTransactionCashFlowResponse struct {
	CashFlow []CashFlowDataPoint `json:"cash_flow"`
}

type CashFlowRow struct {
	Month        time.Time
	TotalAmount  string
	InvoiceCount int
}

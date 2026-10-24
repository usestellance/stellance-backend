package invoice

// InvoiceStatsResponse is the data of GET /invoice/stats: invoice counts per status for the signed-in user.
type InvoiceStatsResponse struct {
	PendingCount        int `db:"pending_count" json:"pending_count" example:"4"`
	PaidCount           int `db:"paid_count" json:"paid_count" example:"12"`
	OverdueCount        int `db:"overdue_count" json:"overdue_count" example:"0"`
	ExpiredCount        int `db:"expired_count" json:"expired_count" example:"2"`
	InvalidPaymentCount int `db:"invalid_payment_count" json:"invalid_payment_count" example:"1"`
	TotalCount          int `db:"total_count" json:"total_count" example:"19"`
}

// InvoiceSendResult is the data of POST /invoice/send/{id}.
type InvoiceSendResult struct {
	TotalRecipients int    `json:"total_recipients" example:"2"`
	InvoiceURL      string `json:"invoice_url" example:"aB3dE9"`
}

// InvoiceUpdateResult is the data of the invoice update endpoints.
type InvoiceUpdateResult struct {
	InvoiceID string `json:"invoice_id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"` // Updated invoice
	UpdatedAt string `json:"updated_at" example:"2026-10-22T14:03:26Z"`                 // Update time
}

// InvoiceSearchResult is the data of GET /invoice/query.
type InvoiceSearchResult struct {
	Invoice []InvoiceResponse    `json:"invoice"`
	Meta    SearchPaginationMeta `json:"meta"`
}

// MarkPaidResult is the data of PATCH /invoice/{id}/mark-paid. On 422 it carries the payments that were checked instead.
type MarkPaidResult struct {
	InvoiceID string `json:"invoice_id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"` // Invoice id
	Status    string `json:"status" example:"paid"`
	Payment   any    `json:"payment,omitempty"` // The verified payment record
}

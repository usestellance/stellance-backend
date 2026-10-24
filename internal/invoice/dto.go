package invoice

import (
	"time"

	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
)

type InvoiceType string
type InvoiceStatus string
type TemplateIDType string

const (
	PerUnit                      InvoiceType    = "per_unit"
	PerHour                      InvoiceType    = "per_hour"
	InvoiceStatusDraft           InvoiceStatus  = "draft"
	InvoiceStatusSent            InvoiceStatus  = "sent"
	InvoiceStatusViewed          InvoiceStatus  = "viewed"
	InvoiceStatusPaid            InvoiceStatus  = "paid"
	InvoiceStatusOverdue         InvoiceStatus  = "overdue"
	InvoiceStatusCancelled       InvoiceStatus  = "cancelled"
	InvoiceStatusRefunded        InvoiceStatus  = "refunded"
	InvoiceStatusPending         InvoiceStatus  = "pending"
	InvoiceStatusPaymentDetected InvoiceStatus  = "payment_detected"
	InvoiceStatusInvalidPayment  InvoiceStatus  = "invalid_payment"
	InvoiceStatusExpired         InvoiceStatus  = "expired"
	TemplateID1                  TemplateIDType = "template_001"
	TemplateID2                  TemplateIDType = "template_002"
	TemplateID3                  TemplateIDType = "template_003"
	TemplateID4                  TemplateIDType = "template_004"
	TemplateID5                  TemplateIDType = "template_005"
)

type CreateInvoiceDTO struct {
	Title         string         `json:"title" validate:"required,min=3" example:"Website redesign"`            // Invoice title, min 3 characters
	RecipientName string         `json:"payer_name" validate:"required" example:"Acme Ltd"`                     // Client name
	Email         string         `json:"payer_email" validate:"required,email" example:"billing@acme.com"`      // Client email the invoice is sent to
	Country       string         `json:"country" validate:"required" example:"NG"`                              // Client country
	InvoiceItems  []InvoiceItems `json:"invoice_items" validate:"required,min=1"`                               // At least one line item
	ServiceFee    float64        `json:"service_fee" validate:"gte=0" example:"2.5"`                            // Extra fee added to the subtotal, >= 0
	DueDate       string         `json:"due_date" validate:"required,datetime=2006-01-02" example:"2026-12-31"` // YYYY-MM-DD. Payments are accepted until the end of this day (UTC) plus PAYMENT_EXPIRY_GRACE_DAYS
	TemplateID    TemplateIDType `json:"template_id" validate:"required" example:"template_001"`                // One of template_001..template_005
	MakeDefault   bool           `json:"make_default,omitempty" example:"false"`                                // Save this template as the default
	Note          string         `json:"note,omitempty" example:"Thanks for your business"`                     // Optional note shown on the invoice
}

type InvoiceItems struct {
	ItemId      string      `json:"item_id,omitempty"`                                                           // Existing item id when editing; omit for new items
	InvoiceType InvoiceType `json:"invoice_type" validate:"required,oneof=per_hour per_unit" example:"per_unit"` // per_hour or per_unit
	Description string      `json:"description" validate:"required" example:"Design work"`                       // What was delivered
	Quantity    int64       `json:"quantity" validate:"required,gt=0" example:"3"`                               // Hours or units, > 0
	UnitPrice   float64     `json:"unit_price" validate:"required,gt=0" example:"150.5"`                         // Price per hour or unit, > 0
	Discount    int64       `json:"discount,omitempty" example:"0"`                                              // Discount percent
	Amount      float64     `json:"amount" validate:"required,gt=0" example:"451.5"`                             // Line total; the server checks it against quantity x unit_price
	CreatedAt   *time.Time  `json:"created_at,omitempty" example:"2026-10-20T09:00:00Z"`                         // When the item was added
}

type InvoiceResponse struct {
	ID                 string               `json:"id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"`                                                 // Invoice id
	InvoiceNumber      string               `json:"invoice_number" example:"INV-20261020-0019"`                                                        // Human-readable number
	InvoiceURL         string               `json:"invoice_url" example:"aB3dE9"`                                                                      // Short public url used by /invoice/search and /payments/request
	Title              string               `json:"title,omitempty" example:"Website redesign"`                                                        // Title
	PayerEmail         string               `json:"payer_email" example:"billing@acme.com"`                                                            // Client email
	PayerName          string               `json:"payer_name,omitempty" example:"Acme Ltd"`                                                           // Client name
	PayerWalletAddress string               `json:"payer_wallet_address,omitempty" example:"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"` // Account that paid; set once paid
	Country            string               `json:"country,omitempty" example:"NG"`                                                                    // Client country
	SubTotal           float64              `json:"sub_total" example:"451.5"`                                                                         // Sum of line items
	ServiceFee         float64              `json:"service_fee" example:"2.5"`                                                                         // Fee added to the subtotal
	Total              float64              `json:"total" example:"454"`                                                                               // Amount due
	Currency           string               `json:"currency" example:"usdc"`                                                                           // usdc or xlm
	Status             string               `json:"status" example:"sent"`                                                                             // draft, sent, viewed, pending, overdue, payment_detected, invalid_payment, paid, expired, cancelled or refunded
	DueDate            time.Time            `json:"due_date" example:"2026-12-31T00:00:00Z"`                                                           // Due date
	CreatedAt          time.Time            `json:"created_at" example:"2026-10-20T09:00:00Z"`                                                         // Creation time
	UpdatedAt          time.Time            `json:"updated_at,omitempty" example:"2026-10-22T14:03:26Z"`                                               // Last change
	PaidAt             *time.Time           `json:"paid_at,omitempty" example:"2026-10-22T14:03:26Z"`                                                  // Set once paid
	Items              []InvoiceItems       `json:"items"`
	CreatedBy          InvoiceSenderDetails `json:"createdBy"`
	Approved           *bool                `json:"approved" example:"true"`                                // Payer review result; null before review
	ReviewDate         *time.Time           `json:"review_date" example:"2026-10-21T10:00:00Z"`             // When the payer approved or rejected
	TemplateID         TemplateIDType       `json:"template_id" validate:"required" example:"template_001"` // Template
	LogoURL            string               `json:"logo_url,omitempty"`                                     // Signed logo url
	Note               string               `json:"note,omitempty"`                                         // Note shown on the invoice
	PaymentStatus      string               `json:"payment_status" example:"awaiting_payment"`              // Derived from status; see docs/payment-status-model.md
	PaymentReference   string               `json:"payment_reference" example:"STL-B9371BD3A173"`           // Memo to attach to the on-chain payment
}

type InvoiceListResponseDto struct {
	Items   []InvoiceResponse `json:"items"`
	Invoice []InvoiceResponse `json:"invoice"` // deprecated: use items
	Meta    PaginationMeta    `json:"meta"`
}

type PaginationMeta struct {
	Page              int `json:"page" example:"1"`        // Current page
	Limit             int `json:"limit" example:"10"`      // Page size
	Total             int `json:"total" example:"57"`      // Total matching invoices
	TotalPages        int `json:"total_pages" example:"6"` // Total pages
	PageCount         int `json:"page_count,omitempty"`    // deprecated: use limit
	TotalInvoiceCount int `json:"total_invoice_count"`     // deprecated: use total
}

type SearchPaginationMeta struct {
	Total   int  `json:"total" example:"57"`       // Total matches
	Limit   int  `json:"limit" example:"10"`       // Page size
	Offset  int  `json:"offset" example:"0"`       // Rows skipped
	HasNext bool `json:"has_next" example:"true"`  // More rows after this page
	HasPrev bool `json:"has_prev" example:"false"` // Rows before this page
}

type InvoiceFiltersDto struct {
	Status  InvoiceStatus     `json:"status,omitempty" validate:"omitempty,invoice_status"`
	UserId  string            `json:"user_id,omitempty"`
	Page    int               `json:"page" validate:"required,min=1"`
	Count   int               `json:"count" validate:"required,min=1,max=15"`
	OrderBy utils.OrderByType `json:"order_by" validate:"required,order_by"`
	Search  string            `json:"search,omitempty"`
}

type InvoiceSenderDetails struct {
	UserId         string  `json:"user_id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"`                                      // Creator id
	Name           string  `json:"name" example:"Jane Doe"`                                                                     // Creator name
	Email          string  `json:"email,omitempty" example:"jane@example.com"`                                                  // Hidden on the public view
	Location       string  `json:"location" example:"NG"`                                                                       // Creator country
	BusinessName   *string `json:"business_name,omitempty" example:"Doe Studio"`                                                // Business name
	PhoneNumber    *string `json:"phone_number,omitempty" example:"+2348012345678"`                                             // Hidden on the public view
	Wallet_address *string `json:"wallet_address,omitempty" example:"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"` // Creator primary wallet
}

type InvoiceStatusQuery struct {
	Month string `form:"month" json:"month"`
}

type InvoiceStatusDataPoint struct {
	Status string `json:"status" example:"Paid"` // Capitalised status name
	Value  int    `json:"value" example:"12"`    // Invoices in this status
}

type GetInvoiceByStatusResponse struct {
	InvoicesByStatus []InvoiceStatusDataPoint `json:"invoices_by_status"`
}

type InvoiceStatusRow struct {
	Status string
	Count  int
}

type SendInvoiceEmailData struct {
	PrimaryRecipient string
	CCRecipients     []string
	PayerName        string
	SenderName       string
	InvoiceURL       string
}

type SendInvoiceDto struct {
	Emails []string `json:"emails" validate:"required,min=1,max=10,dive,email"` // 1-10 recipient emails; the first is the primary recipient, the rest are CC
}

type MarkInvoicePaidDTO struct {
	TransactionHash string `json:"transaction_hash" validate:"required,len=64,hexadecimal" example:"a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7"` // 64-character hex Stellar transaction hash. The server re-verifies it on-chain
}

type PDFInvoiceData struct {
	InvoiceNumber   string
	Status          string
	StatusClass     string
	CreatorName     string
	CreatorEmail    string
	CreatorBusiness string
	CreatorPhone    string
	CreatorCountry  string
	PayerName       string
	PayerEmail      string
	PayerCountry    string
	Currency        string
	SubTotal        float64
	ServiceFee      float64
	Total           float64
	CreatedAt       string
	DueDate         string
	GeneratedAt     string
	Note            string
	Items           []InvoiceItems
}

type UpdateInvoiceDTO struct {
	Title         string         `json:"title" validate:"omitempty,min=3" example:"Website redesign"`            // Min 3 characters
	RecipientName string         `json:"payer_name" validate:"omitempty" example:"Acme Ltd"`                     // Client name
	Email         string         `json:"payer_email" validate:"omitempty,email" example:"billing@acme.com"`      // Client email
	Country       string         `json:"country" validate:"omitempty" example:"NG"`                              // Client country
	InvoiceItems  []InvoiceItems `json:"invoice_items" validate:"omitempty,min=1"`                               // Replaces the item list
	ServiceFee    *float64       `json:"service_fee" validate:"omitempty,gte=0" example:"2.5"`                   // >= 0
	DueDate       string         `json:"due_date" validate:"omitempty,datetime=2006-01-02" example:"2026-12-31"` // YYYY-MM-DD
	TemplateID    TemplateIDType `json:"template_id" validate:"omitempty" example:"template_002"`                // One of template_001..template_005
}

type ReviewInvoiceDTO struct {
	Approve *bool `json:"approve" example:"true"` // true approves, false rejects (cancels) the invoice
}

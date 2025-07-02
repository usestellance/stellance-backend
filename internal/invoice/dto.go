package invoice

import (
	"time"

	"github.com/The-True-Hooha/stellance-backend.git/pkg/utils"
)

type InvoiceType string
type InvoiceStatus string

const (
	PerUnit                InvoiceType   = "per_unit"
	PerHour                InvoiceType   = "per_hour"
	InvoiceStatusDraft     InvoiceStatus = "draft"
	InvoiceStatusSent      InvoiceStatus = "sent"
	InvoiceStatusViewed    InvoiceStatus = "viewed"
	InvoiceStatusPaid      InvoiceStatus = "paid"
	InvoiceStatusOverdue   InvoiceStatus = "overdue"
	InvoiceStatusCancelled InvoiceStatus = "cancelled"
	InvoiceStatusRefunded  InvoiceStatus = "refunded"
	InvoiceStatusPending   InvoiceStatus = "pending"
)

type CreateInvoiceDTO struct {
	Title         string         `json:"title," validation:"required,min=3"`
	RecipientName string         `json:"payer_name" validate:"required"`
	Email         string         `json:"payer_email" validate:"required,email"`
	Country       string         `json:"country" validate:"required"`
	InvoiceItems  []InvoiceItems `json:"invoice_items" validate:"required,min=1"`
	ServiceFee    float64        `json:"service_fee" validate:"gte=0"`
	DueDate       string         `json:"due_date" validate:"required,datetime=2006-01-02"`
}

type InvoiceItems struct {
	ItemId      string      `json:"item_id,omitempty"`
	InvoiceType InvoiceType `json:"invoice_type" validate:"required,oneof=per_hour per_unit"`
	Description string      `json:"description" validate:"required"`
	Quantity    int64       `json:"quantity" validate:"required,gt=0"`
	UnitPrice   float64     `json:"unit_price" validate:"required,gt=0"`
	Discount    int64       `json:"discount,omitempty"`
	Amount      float64     `json:"amount" validate:"required,gt=0"`
	CreatedAt   *time.Time  `json:"created_at,omitempty"`
}

type InvoiceResponse struct {
	ID                 string               `json:"id"`
	InvoiceNumber      string               `json:"invoice_number"`
	InvoiceURL         string               `json:"invoice_url"`
	Title              string               `json:"title,omitempty"`
	PayerEmail         string               `json:"payer_email"`
	PayerName          string               `json:"payer_name,omitempty"`
	PayerWalletAddress string               `json:"payer_wallet_address,omitempty"`
	Country            string               `json:"country,omitempty"`
	SubTotal           float64              `json:"sub_total"`
	ServiceFee         float64              `json:"service_fee"`
	Total              float64              `json:"total"`
	Currency           string               `json:"currency"`
	Status             string               `json:"status"`
	DueDate            time.Time            `json:"due_date"`
	CreatedAt          time.Time            `json:"created_at"`
	UpdatedAt          time.Time            `json:"updated_at,omitempty"`
	PaidAt             *time.Time           `json:"paid_at,omitempty"`
	Items              []InvoiceItems       `json:"items"`
	CreatedBy          InvoiceSenderDetails `json:"createdBy"`
}

type InvoiceListResponseDto struct {
	Invoice []InvoiceResponse `json:"invoice"`
	Meta    PaginationMeta    `json:"meta"`
}

type PaginationMeta struct {
	Page              int `json:"page"`
	PageCount         int `json:"page_count,omitempty"`
	TotalInvoiceCount int `json:"total_invoice_count"`
	TotalPages        int `json:"total_pages"`
}

type InvoiceFiltersDto struct {
	Status  InvoiceStatus     `json:"status,omitempty" validate:"omitempty,invoice_status"`
	UserId  string            `json:"user_id,omitempty"`
	Page    int               `json:"page" validate:"required,min=1"`
	Count   int               `json:"count" validate:"required,min=1,max=15"`
	OrderBy utils.OrderByType `json:"order_by" validate:"required,order_by"`
}

type InvoiceSenderDetails struct {
	UserId       string  `json:"user_id"`
	Name         string  `json:"name"`
	Email        string  `json:"email"`
	Location     string  `json:"location"`
	BusinessName *string `json:"business_name,omitempty"`
	PhoneNumber  *string `json:"phone_number,omitempty"`
}

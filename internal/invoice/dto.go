package invoice

import "time"

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
	InvoiceType InvoiceType `json:"invoice_type" validate:"required,oneof=per_hour per_unit"`
	Description string      `json:"description" validate:"required"`
	Quantity    int64       `json:"quantity" validate:"required,gt=0"`
	UnitPrice   float64     `json:"unit_price" validate:"required,gt=0"`
	Discount    int64       `json:"discount,omitempty"`
	Amount      float64     `json:"amount" validate:"required,gt=0"`
}

type InvoiceResponse struct {
	ID            string         `json:"id"`
	InvoiceNumber string         `json:"invoice_number"`
	InvoiceURL    string         `json:"invoice_url"`
	Title         string         `json:"title,omitempty"`
	PayerEmail    string         `json:"payer_email"`
	PayerName     string         `json:"payer_name"`
	Country       string         `json:"country"`
	SubTotal      float64        `json:"sub_total"`
	ServiceFee    float64        `json:"service_fee"`
	Total         float64        `json:"total"`
	Currency      string         `json:"currency"`
	Status        string         `json:"status"`
	DueDate       string         `json:"due_date"`
	CreatedAt     time.Time      `json:"created_at"`
	Items         []InvoiceItems `json:"items"`
}

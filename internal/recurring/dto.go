package recurring

import (
	"time"

	"github.com/The-True-Hooha/stellance-backend/internal/invoice"
)

type RecurringInterval string

const (
	IntervalWeekly    RecurringInterval = "weekly"
	IntervalBiweekly  RecurringInterval = "biweekly"
	IntervalMonthly   RecurringInterval = "monthly"
	IntervalQuarterly RecurringInterval = "quarterly"
	IntervalYearly    RecurringInterval = "yearly"
)

type RecurringInvoice struct {
	ID           string                 `json:"id"`
	UserID       string                 `json:"user_id"`
	Title        string                 `json:"title"`
	PayerEmail   string                 `json:"payer_email"`
	PayerName    string                 `json:"payer_name"`
	Country      string                 `json:"country"`
	Currency     string                 `json:"currency"`
	SubTotal     float64                `json:"sub_total"`
	ServiceFee   float64                `json:"service_fee"`
	Total        float64                `json:"total"`
	TemplateID   string                 `json:"template_id"`
	Note         string                 `json:"note,omitempty"`
	Interval     RecurringInterval      `json:"interval"`
	NextRunAt    time.Time              `json:"next_run_at"`
	LastRunAt    *time.Time             `json:"last_run_at,omitempty"`
	IsActive     bool                   `json:"is_active"`
	InvoiceItems []invoice.InvoiceItems `json:"invoice_items"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

type CreateRecurringDTO struct {
	Title        string                 `json:"title" validate:"required,min=3"`
	PayerEmail   string                 `json:"payer_email" validate:"required,email"`
	PayerName    string                 `json:"payer_name" validate:"required"`
	Country      string                 `json:"country" validate:"required"`
	InvoiceItems []invoice.InvoiceItems `json:"invoice_items" validate:"required,min=1"`
	ServiceFee   float64                `json:"service_fee" validate:"gte=0"`
	TemplateID   string                 `json:"template_id" validate:"required"`
	Note         string                 `json:"note,omitempty"`
	Interval     RecurringInterval      `json:"interval" validate:"required,oneof=weekly biweekly monthly quarterly yearly"`
	StartDate    string                 `json:"start_date" validate:"required,datetime=2006-01-02"`
}

type UpdateRecurringDTO struct {
	IsActive  *bool             `json:"is_active"`
	Interval  RecurringInterval `json:"interval" validate:"omitempty,oneof=weekly biweekly monthly quarterly yearly"`
	StartDate string            `json:"start_date" validate:"omitempty,datetime=2006-01-02"`
}

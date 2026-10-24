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
	ID           string                 `json:"id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"`      // Schedule id
	UserID       string                 `json:"user_id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"` // Owner
	Title        string                 `json:"title" example:"Monthly retainer"`                       // Title
	PayerEmail   string                 `json:"payer_email" example:"billing@acme.com"`                 // Client email
	PayerName    string                 `json:"payer_name" example:"Acme Ltd"`                          // Client
	Country      string                 `json:"country" example:"NG"`                                   // Country
	Currency     string                 `json:"currency" example:"usdc"`                                // Currency
	SubTotal     float64                `json:"sub_total" example:"451.5"`                              // Sum of items
	ServiceFee   float64                `json:"service_fee" example:"0"`                                // Fee
	Total        float64                `json:"total" example:"451.5"`                                  // Total per invoice
	TemplateID   string                 `json:"template_id" example:"template_001"`                     // Template
	Note         string                 `json:"note,omitempty"`                                         // Note
	Interval     RecurringInterval      `json:"interval" example:"monthly"`                             // Schedule interval
	NextRunAt    time.Time              `json:"next_run_at" example:"2026-11-01T00:00:00Z"`             // Next invoice time
	LastRunAt    *time.Time             `json:"last_run_at,omitempty" example:"2026-10-01T00:00:00Z"`   // Last invoice time
	IsActive     bool                   `json:"is_active" example:"true"`                               // False when paused
	InvoiceItems []invoice.InvoiceItems `json:"invoice_items"`
	CreatedAt    time.Time              `json:"created_at" example:"2026-09-01T00:00:00Z"` // Created
	UpdatedAt    time.Time              `json:"updated_at" example:"2026-10-01T00:00:00Z"` // Updated
}

type CreateRecurringDTO struct {
	Title        string                 `json:"title" validate:"required,min=3" example:"Monthly retainer"`                                    // Title copied to each generated invoice
	PayerEmail   string                 `json:"payer_email" validate:"required,email" example:"billing@acme.com"`                              // Client email
	PayerName    string                 `json:"payer_name" validate:"required" example:"Acme Ltd"`                                             // Client name
	Country      string                 `json:"country" validate:"required" example:"NG"`                                                      // Client country
	InvoiceItems []invoice.InvoiceItems `json:"invoice_items" validate:"required,min=1"`                                                       // Line items copied to each invoice
	ServiceFee   float64                `json:"service_fee" validate:"gte=0" example:"0"`                                                      // >= 0
	TemplateID   string                 `json:"template_id" validate:"required" example:"template_001"`                                        // Invoice template
	Note         string                 `json:"note,omitempty"`                                                                                // Optional note
	Interval     RecurringInterval      `json:"interval" validate:"required,oneof=weekly biweekly monthly quarterly yearly" example:"monthly"` // weekly, biweekly, monthly, quarterly or yearly
	StartDate    string                 `json:"start_date" validate:"required,datetime=2006-01-02" example:"2026-11-01"`                       // YYYY-MM-DD of the first generated invoice
}

type UpdateRecurringDTO struct {
	IsActive  *bool             `json:"is_active" example:"false"`                                                                      // false pauses the schedule, true resumes it
	Interval  RecurringInterval `json:"interval" validate:"omitempty,oneof=weekly biweekly monthly quarterly yearly" example:"monthly"` // New interval
	StartDate string            `json:"start_date" validate:"omitempty,datetime=2006-01-02" example:"2026-12-01"`                       // YYYY-MM-DD, resets the next run date
}

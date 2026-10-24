package notifications

import (
	"time"

	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
)

type CreateNotificationDto struct {
	UserId string
	Title  string
	Body   string
}

type Notification struct {
	Id       string     `json:"id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"`                 // Notification id
	Title    string     `json:"title" example:"Invoice Paid"`                                      // Short title
	Body     string     `json:"body" example:"Payment for invoice INV-1 was verified on Stellar."` // Message
	Viewed   bool       `json:"viewed" example:"false"`                                            // Read flag
	ViewedAt *time.Time `json:"viewed_at,omitempty" example:"2026-10-22T14:10:00Z"`                // When it was read
}

type Notifications struct {
	Notification []Notification `json:"notifications"`
	UnreadCount  int32          `json:"unread_count"`
	ReadCount    int32          `json:"read_count"`
	TotalCount   int32          `json:"total_count"`
}

type GetNotificationsQuery struct {
	Page    int               `json:"page" validate:"required,min=1"`
	Count   int               `json:"count" validate:"required,min=1,max=15"`
	OrderBy utils.OrderByType `json:"order_by" validate:"required,order_by"`
	Viewed  *bool             `json:"viewed"`
}

type PaginationMeta struct {
	Page        int `json:"page" example:"1"`                  // Current page
	Limit       int `json:"limit" example:"15"`                // Page size
	Total       int `json:"total" example:"42"`                // Notifications matching the viewed filter
	PageCount   int `json:"page_count,omitempty" example:"15"` // Deprecated: items on this page
	UnreadCount int `json:"unread_count" example:"5"`          // All unread notifications
	ReadCount   int `json:"read_count" example:"37"`           // All read notifications
	TotalPages  int `json:"total_pages" example:"3"`           // Total pages
}

type GetNotificationResponse struct {
	Items         []Notification `json:"items"`
	Notifications []Notification `json:"notifications"` // deprecated: use items
	Meta          PaginationMeta `json:"meta"`
}

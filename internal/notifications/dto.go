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
	Id       string     `json:"id"`
	Title    string     `json:"title"`
	Body     string     `json:"body"`
	Viewed   bool       `json:"viewed"`
	ViewedAt *time.Time `json:"viewed_at,omitempty"`
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
	Page        int `json:"page"`
	PageCount   int `json:"page_count,omitempty"`
	UnreadCount int `json:"unread_count"`
	ReadCount   int `json:"read_count"`
	TotalPages  int `json:"total_pages"`
}

type GetNotificationResponse struct {
	Notifications []Notification `json:"notifications"`
	Meta          PaginationMeta `json:"meta"`
}

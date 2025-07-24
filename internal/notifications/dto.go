package notifications

import "time"

type CreateNotificationDto struct {
	UserId string
	Title  string
	Body   string
}

type Notifications struct {
	Id          string     `json:"id"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	Viewed      bool       `json:"viewed"`
	ViewedAt    *time.Time `json:"viewed_at,omitempty"`
	UnreadCount int32      `json:"unread_count"`
	ReadCount   int32      `json:"read_count"`
	TotalCount  int32      `json:"total_count"`
}

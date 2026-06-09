package invoice_comments

import (
	"database/sql"
	"time"
)

type InvoiceComment struct {
	ID              string         `json:"id" db:"id"`
	InvoiceID       string         `json:"invoice_id" db:"invoice_id"`
	UserID          sql.NullString `json:"user_id,omitempty" db:"user_id"`
	CommenterName   string         `json:"commenter_name" db:"commenter_name"`
	CommenterEmail  string         `json:"commenter_email" db:"commenter_email"`
	CommentText     string         `json:"comment_text" db:"comment_text"`
	IsVerified      bool           `json:"is_verified" db:"is_verified"`
	IsGuest         bool           `json:"is_guest" db:"is_guest"`
	ParentCommentID sql.NullString `json:"parent_comment_id,omitempty" db:"parent_comment_id"`
	Edited          bool           `json:"edited" db:"edited"`
	EditedAt        sql.NullTime   `json:"edited_at,omitempty" db:"edited_at"`
	CreatedAt       time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at" db:"updated_at"`
	ParentID        sql.NullString `json:"parent_id,omitempty" db:"parent_comment_id"`
}

type InvoiceCommentStats struct {
	InvoiceID        string    `json:"invoice_id" db:"invoice_id"`
	TotalComments    int       `json:"total_comments" db:"total_comments"`
	VerifiedComments int       `json:"verified_comments" db:"verified_comments"`
	GuestComments    int       `json:"guest_comments" db:"guest_comments"`
	TopLevelComments int       `json:"top_level_comments" db:"top_level_comments"`
	ReplyComments    int       `json:"reply_comments" db:"reply_comments"`
	LatestCommentAt  time.Time `json:"latest_comment_at" db:"latest_comment_at"`
}

type CreateCommentDTO struct {
	Token          string `json:"token" validate:"omitempty"`
	InvoiceID      string `json:"invoice_id" validate:"required,uuid"`
	CommentText    string `json:"comment_text" validate:"required,min=1,max=2000"`
	ParentID       string `json:"parent_id,omitempty" validate:"omitempty,uuid"`
	// CommenterEmail string `json:"commenter_email,omitempty" validate:"omitempty,email"`
}

type UpdateCommentDTO struct {
	CommentText string `json:"comment_text" validate:"required,min=1,max=2000"`
	Email       string `json:"email,omitempty" validate:"omitempty,email"`
	Token       string `json:"token,omitempty" validate:"omitempty"`
}

type GetCommentsQuery struct {
	InvoiceID  string `form:"invoice_id" validate:"required,uuid"`
	ParentID   string `form:"parent_id"`
	IsVerified *bool  `form:"is_verified"`
	Page       int    `form:"page"`
	Limit      int    `form:"limit"`
	SortOrder  string `form:"sort_order"`
}

type CommentResponse struct {
	InvoiceComment
	Replies   []CommentResponse `json:"replies,omitempty"`
	CanEdit   bool              `json:"can_edit"`
	CanDelete bool              `json:"can_delete"`
	Reactions []ReactionCount   `json:"reactions,omitempty"`
}

type GetCommentsResponse struct {
	Comments   []CommentResponse   `json:"comments"`
	Stats      InvoiceCommentStats `json:"stats,omitempty"`
	Total      int                 `json:"total"`
	Page       int                 `json:"page"`
	Limit      int                 `json:"limit"`
	TotalPages int                 `json:"total_pages"`
}

type CreateCommentQueryResponse struct {
	ID         string
	Created_at time.Time
	Verified   bool
	Guest      bool
}

type GetInvoiceDto struct {
	ID            string
	UserID        string
	InvoiceURL    string
	InvoiceNumber string
	InvoiceStatus string
}

type GetUserDto struct {
	UserId    string
	FirstName string
	LastName  string
	Email     string
}

type ReactToCommentDTO struct {
	Emoji string `json:"emoji" validate:"required,min=1,max=10"`
	Email string `json:"email,omitempty" validate:"omitempty,email"`
	Token string `json:"token,omitempty"`
}

type ReactionCount struct {
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
	// Reacted is true if the current viewer has reacted with this emoji
	Reacted bool `json:"reacted"`
}

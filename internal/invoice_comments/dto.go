package invoice_comments

import (
	"database/sql"
	"time"
)

type InvoiceComment struct {
	ID              string         `json:"id" db:"id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"`                 // Comment id
	InvoiceID       string         `json:"invoice_id" db:"invoice_id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"` // Invoice id
	UserID          sql.NullString `json:"user_id,omitempty" db:"user_id" swaggertype:"object"`                       // Author id; empty for guests
	CommenterName   string         `json:"commenter_name" db:"commenter_name" example:"Jane Doe"`                     // Display name
	CommenterEmail  string         `json:"commenter_email" db:"commenter_email" example:"jane@example.com"`           // Author email
	CommentText     string         `json:"comment_text" db:"comment_text" example:"Looks good"`                       // Comment body
	IsVerified      bool           `json:"is_verified" db:"is_verified" example:"true"`                               // Author is a verified account
	IsGuest         bool           `json:"is_guest" db:"is_guest" example:"false"`                                    // Author commented with a guest token
	ParentCommentID sql.NullString `json:"parent_comment_id,omitempty" db:"parent_comment_id" swaggertype:"object"`   // Parent comment when this is a reply
	Edited          bool           `json:"edited" db:"edited" example:"false"`                                        // Edited after posting
	EditedAt        sql.NullTime   `json:"edited_at,omitempty" db:"edited_at" swaggertype:"object"`                   // When last edited
	CreatedAt       time.Time      `json:"created_at" db:"created_at" example:"2026-10-22T14:03:26Z"`                 // Posted
	UpdatedAt       time.Time      `json:"updated_at" db:"updated_at" example:"2026-10-22T14:03:26Z"`                 // Last change
	ParentID        sql.NullString `json:"parent_id,omitempty" db:"parent_comment_id" swaggertype:"object"`           // Same as parent_comment_id
}

type InvoiceCommentStats struct {
	InvoiceID        string    `json:"invoice_id" db:"invoice_id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"` // Invoice id
	TotalComments    int       `json:"total_comments" db:"total_comments" example:"8"`                            // All comments
	VerifiedComments int       `json:"verified_comments" db:"verified_comments" example:"5"`                      // By verified accounts
	GuestComments    int       `json:"guest_comments" db:"guest_comments" example:"3"`                            // By guests
	TopLevelComments int       `json:"top_level_comments" db:"top_level_comments" example:"6"`                    // Not replies
	ReplyComments    int       `json:"reply_comments" db:"reply_comments" example:"2"`                            // Replies
	LatestCommentAt  time.Time `json:"latest_comment_at" db:"latest_comment_at" example:"2026-10-22T14:03:26Z"`   // Most recent comment
}

type CreateCommentDTO struct {
	Token       string `json:"token" validate:"omitempty"`                                                                      // Invoice access token for guests; omit when signed in
	InvoiceID   string `json:"invoice_id" validate:"required,uuid" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"`              // Invoice being discussed
	CommentText string `json:"comment_text" validate:"required,min=1,max=2000" example:"Can you split this into two payments?"` // 1-2000 characters
	ParentID    string `json:"parent_id,omitempty" validate:"omitempty,uuid"`                                                   // Comment id to reply to; omit for a top-level comment
	// CommenterEmail string `json:"commenter_email,omitempty" validate:"omitempty,email"`
}

type UpdateCommentDTO struct {
	CommentText string `json:"comment_text" validate:"required,min=1,max=2000" example:"Updated text"` // 1-2000 characters
	Email       string `json:"email,omitempty" validate:"omitempty,email"`                             // Guest email, required for guests
	Token       string `json:"token,omitempty" validate:"omitempty"`                                   // Guest invoice access token
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
	CanEdit   bool              `json:"can_edit" example:"true"`   // Viewer may edit
	CanDelete bool              `json:"can_delete" example:"true"` // Viewer may delete
	Reactions []ReactionCount   `json:"reactions,omitempty"`
}

type GetCommentsResponse struct {
	Comments   []CommentResponse   `json:"comments"`
	Stats      InvoiceCommentStats `json:"stats,omitempty"`
	Total      int                 `json:"total" example:"8"`       // Total top-level comments
	Page       int                 `json:"page" example:"1"`        // Current page
	Limit      int                 `json:"limit" example:"20"`      // Page size
	TotalPages int                 `json:"total_pages" example:"1"` // Total pages
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
	Emoji string `json:"emoji" validate:"required,min=1,max=10" example:"👍"` // 1-10 characters
	Email string `json:"email,omitempty" validate:"omitempty,email"`         // Guest email
	Token string `json:"token,omitempty"`                                    // Guest invoice access token
}

type ReactionCount struct {
	Emoji string `json:"emoji" example:"👍"` // Emoji
	Count int    `json:"count" example:"3"` // Reactions with this emoji
	// Reacted is true if the current viewer has reacted with this emoji
	Reacted bool `json:"reacted"`
}

package invoice

import "time"

type Logo struct {
	ID        string    `json:"id" db:"id"`
	UserID    string    `json:"user_id" db:"user_id"`
	FileName  string    `json:"file_name" db:"file_name"`
	FileSize  int64     `json:"file_size" db:"file_size"`
	FileType  string    `json:"file_type" db:"file_type"`
	S3Key     string    `json:"s3_key" db:"s3_key"`
	S3Bucket  string    `json:"s3_bucket" db:"s3_bucket"`
	IsDefault bool      `json:"is_default" db:"is_default"`
	Width     *int      `json:"width,omitempty" db:"width"`
	Height    *int      `json:"height,omitempty" db:"height"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type LogoWithURL struct {
	Logo
	URL       string `json:"url"`
	ExpiresIn int    `json:"expires_in"`
}
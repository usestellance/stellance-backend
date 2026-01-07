package logo

import (
	"mime/multipart"
	"time"
)

type Logo struct {
	ID               string    `json:"id" db:"id"`
	UserID           string    `json:"user_id" db:"user_id"`
	FileName         string    `json:"file_name" db:"file_name"`
	FileSize         int64     `json:"file_size" db:"file_size"`
	FileType         string    `json:"file_type" db:"file_type"`
	S3Key            string    `json:"s3_key" db:"s3_key"`
	S3Bucket         string    `json:"s3_bucket" db:"s3_bucket"`
	LogoPresignedURL string    `json:"logo_presigned_url" db:"logo_presigned_url"`
	IsDefault        bool      `json:"is_default" db:"is_default"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}

type CreateLogoDTO struct {
	FileName  string `json:"file_name"`
	FileSize  int64  `json:"file_size"`
	FileType  string `json:"file_type"`
	S3Key     string `json:"s3_key"`
	IsDefault bool   `json:"is_default"`
}

type LogoFileData struct {
	File        multipart.File
	FileHeader  *multipart.FileHeader
	MakeDefault bool
}

type CreateLogoResponse struct {
	LogoUrl string
	LogoID  string
}

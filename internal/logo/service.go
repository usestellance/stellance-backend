package logo

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/The-True-Hooha/stellance-backend/internal/storage"
	"github.com/The-True-Hooha/stellance-backend/pkg/config"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type LogoService struct {
	log      *slog.Logger
	postgres *pgxpool.Pool
	redis    *redis.Client
	storage  *storage.S3Storage
}

func NewLogoService() *LogoService {
	return &LogoService{
		log:      config.GetAppContainer().Log,
		postgres: config.GetAppContainer().Postgres,
		redis:    config.GetAppContainer().Redis,
		storage:  config.GetAppContainer().Storage,
	}
}

func (lr *LogoService) GetLogoByID(ctx context.Context, logoID string) (*Logo, error) {
	query := `
		SELECT id, user_id, file_name, file_size, file_type, 
		       s3_key, s3_bucket, logo_presigned_url, is_default, 
		       created_at, updated_at
		FROM logos
		WHERE id = $1
	`

	var logo Logo
	err := lr.postgres.QueryRow(ctx, query, logoID).Scan(
		&logo.ID,
		&logo.UserID,
		&logo.FileName,
		&logo.FileSize,
		&logo.FileType,
		&logo.S3Key,
		&logo.S3Bucket,
		&logo.LogoPresignedURL,
		&logo.IsDefault,
		&logo.CreatedAt,
		&logo.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("logo not found")
		}
		return nil, fmt.Errorf("failed to get logo: %w", err)
	}

	return &logo, nil
}

func (lr *LogoService) AddLogoToDatabase(ctx context.Context, tx pgx.Tx, userID string, logo CreateLogoDTO, bucket string, presignedURL string) (string, error) {
	logoID := uuid.New().String()

	const query = `
		INSERT INTO logos (
			id, user_id, file_name, file_size, file_type, 
			s3_key, s3_bucket, logo_presigned_url, is_default
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`

	var returnedID string
	err := tx.QueryRow(ctx, query,
		logoID,
		userID,
		logo.FileName,
		logo.FileSize,
		logo.FileType,
		logo.S3Key,
		bucket,
		presignedURL,
		logo.IsDefault,
	).Scan(&returnedID)

	if err != nil {
		return "", fmt.Errorf("failed to create logo: %w", err)
	}

	return returnedID, nil
}

func (lr *LogoService) GetDefaultLogoByUserID(ctx context.Context, userID string) (*Logo, error) {
	const query = `
		SELECT id, user_id, file_name, file_size, file_type, 
		       s3_key, s3_bucket, logo_presigned_url, is_default, 
		       created_at, updated_at
		FROM logos
		WHERE user_id = $1 AND is_default = TRUE
		LIMIT 1
	`

	var logo Logo
	err := lr.postgres.QueryRow(ctx, query, userID).Scan(
		&logo.ID,
		&logo.UserID,
		&logo.FileName,
		&logo.FileSize,
		&logo.FileType,
		&logo.S3Key,
		&logo.S3Bucket,
		&logo.LogoPresignedURL,
		&logo.IsDefault,
		&logo.CreatedAt,
		&logo.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get default logo: %w", err)
	}

	return &logo, nil
}

func (ls *LogoService) UploadAndCreateLogo(ctx context.Context, tx pgx.Tx, userID string, logoFile *LogoFileData) (*CreateLogoResponse, error) {
	fileHeader := make([]byte, 512)
	if _, err := logoFile.File.Read(fileHeader); err != nil {
		return nil, fmt.Errorf("Error reading file header: %w", err)
	}

	contentType := http.DetectContentType(fileHeader)

	if contentType == "" {
		contentType = "application/octet-stream"
	}

	logoUpload, err := ls.storage.Upload(ctx, storage.UploadInput{
		File:        logoFile.File,
		Filename:    logoFile.FileHeader.Filename,
		ContentType: contentType,
		Folder:      "logos",
	})

	if err != nil {
		ls.log.Error("failed to upload avatar to S3",
			"user_id", userID,
			"error", err,
		)
		return nil, fmt.Errorf("failed to upload to logo buck: %w", err)
	}

	downloadURL, err := ls.storage.GetPresignedDownloadURL(ctx, logoUpload.Key, 150*24*time.Hour)
	if err != nil {
		ls.log.Error("failed to get presigned url for file", "file", logoFile.FileHeader.Filename)
		return nil, fmt.Errorf("failed to generate presigned download URL: %w", err)
	}

	logoDTO := CreateLogoDTO{
		FileName:  logoFile.FileHeader.Filename,
		FileSize:  logoFile.FileHeader.Size,
		FileType:  contentType,
		S3Key:     logoUpload.Key,
		IsDefault: logoFile.MakeDefault,
	}

	logoID, err := ls.AddLogoToDatabase(ctx, tx, userID, logoDTO, logoUpload.Bucket, downloadURL)
	if err != nil {
		ls.log.Error("failed to create logo data to database", "error", err)
		_ = ls.storage.DeleteFile(ctx, logoUpload.Key)
		return nil, err
	}

	return &CreateLogoResponse{
		LogoID:  logoID,
		LogoUrl: downloadURL,
	}, nil
}

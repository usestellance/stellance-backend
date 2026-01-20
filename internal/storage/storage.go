package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/The-True-Hooha/stellance-backend/pkg/logger"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gofrs/uuid"
)

type S3Storage struct {
	client   *s3.Client
	bucket   string
	region   string
	endpoint string
}

type S3Config struct {
	AccessKeyID     string
	SecretAccessKey string
	BucketName      string
	Region          string
	Endpoint        string
}

func NewS3Storage(cfg S3Config) (*S3Storage, error) {
	awsCfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				cfg.AccessKeyID,
				cfg.SecretAccessKey,
				"",
			),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load S3 config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = false
	})

	return &S3Storage{
		client:   client,
		bucket:   cfg.BucketName,
		region:   cfg.Region,
		endpoint: cfg.Endpoint,
	}, nil
}

func (s *S3Storage) GetPresignedUploadURL(ctx context.Context, key, contentType string, expiry time.Duration) (string, error) {
	if expiry == 0 {
		expiry = 15 * time.Minute
	}

	presignClient := s3.NewPresignClient(s.client)

	request, err := presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, s3.WithPresignExpires(expiry))

	if err != nil {
		return "", fmt.Errorf("failed to generate presigned upload URL: %w", err)
	}

	return request.URL, nil
}

func (s *S3Storage) GetPresignedDownloadURL(ctx context.Context, key string) (string, error) {
	presignClient := s3.NewPresignClient(s.client)

	request, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(7*24*time.Hour))

	if err != nil {
		return "", fmt.Errorf("failed to generate presigned download URL: %w", err)
	}

	return request.URL, nil
}

func (s *S3Storage) DeleteFile(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		return fmt.Errorf("failed to delete file from S3: %w", err)
	}

	return nil
}

func (s *S3Storage) Upload(ctx context.Context, input UploadInput) (*UploadResult, error) {
	log := logger.Logger()
	ext := filepath.Ext(input.Filename)
	u7, err := uuid.NewV7()
	if err != nil {
		log.Debug("failed to generate ID for upload: %w", "error", err)
		return nil, fmt.Errorf("failed to generate unique id for upload")
	}
	uniqueID := u7.String()

	key := fmt.Sprintf("%s/%s%s", input.Folder, uniqueID, ext)

	buf := new(bytes.Buffer)
	size, err := io.Copy(buf, input.File)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(buf.Bytes()),
		ContentType: aws.String(input.ContentType),
	})

	if err != nil {
		log.Error("failed to upload to s3", "key", key, "error", err)
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}

	log.Info("file uploaded successfully", "key", key, "size", size)

	return &UploadResult{
		Key:      key,
		Bucket:   s.bucket,
		Size:     size,
		UploadAt: time.Now(),
	}, nil
}
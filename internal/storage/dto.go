package storage

import (
	"io"
	"time"
)

type UploadInput struct {
	File        io.Reader
	Filename    string
	ContentType string
	Folder      string
}

type UploadResult struct {
	Key      string
	Bucket   string
	Size     int64
	UploadAt time.Time
}
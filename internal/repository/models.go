package repository

import "time"

type Avatar struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	FileName  string    `json:"file_name"`
	FileSize  int64     `json:"file_size"`
	MimeType  string    `json:"mime_type"`
	S3Key     string    `json:"s3_key"`
	CreatedAt time.Time `json:"created_at"`
}

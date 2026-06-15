package services

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Service struct {
	Client *minio.Client
	Bucket string
}

func NewS3Service(endpoint, accessKey, secretKey, bucket string) (*S3Service, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: false,
	})
	if err != nil {
		return nil, err
	}

	// Создаём бакет, если не существует
	exists, err := client.BucketExists(context.Background(), bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		err = client.MakeBucket(context.Background(), bucket, minio.MakeBucketOptions{})
		if err != nil {
			return nil, err
		}
	}

	return &S3Service{
		Client: client,
		Bucket: bucket,
	}, nil
}

func (s *S3Service) Upload(key string, reader io.Reader) error {
	_, err := s.Client.PutObject(context.Background(), s.Bucket, key, reader, -1, minio.PutObjectOptions{ContentType: "application/octet-stream"})
	return err
}

func (s *S3Service) Download(key string) (io.ReadCloser, error) {
	obj, err := s.Client.GetObject(context.Background(), s.Bucket, key, minio.GetObjectOptions{})
	return obj, err
}

// Delete удаляет объект из S3‑хранилища по ключу
func (s *S3Service) Delete(key string) error {
	ctx := context.Background()

	err := s.Client.RemoveObject(ctx, s.Bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete object %s from S3: %w", key, err)
	}

	return nil
}

package repository

import (
	"context"
	"fmt"
	"go-avatar-service/internal/domain"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

type AvatarRepository struct {
	DB *pgxpool.Pool
}

func Initialization(path string) (AvatarRepository, error) {
	config, err := pgxpool.ParseConfig(path)
	if err != nil {
		return AvatarRepository{}, fmt.Errorf("failed pgxpool.ParseConfig %w", err)
	}

	// Настройка параметров пула
	config.MaxConns = 20
	config.MinConns = 5
	config.HealthCheckPeriod = 1 * time.Minute
	config.MaxConnIdleTime = 1 * time.Hour
	config.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return AvatarRepository{}, fmt.Errorf("failed pgxpool.NewWithConfig %w", err)
	}

	// Применяем миграции
	if err := runMigrations(pool); err != nil {
		return AvatarRepository{}, err
	}

	return AvatarRepository{DB: pool}, nil
}

func runMigrations(pool *pgxpool.Pool) error {
	// Конвертируем pgxpool.Pool в *sql.DB для совместимости с goose
	sqlDB := stdlib.OpenDBFromPool(pool)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed goose.SetDialect: %w", err)
	}

	if err := goose.Up(sqlDB, "../../migrations"); err != nil {
		return fmt.Errorf("failed goose.Up: %w", err)
	}

	return nil
}

func (r *AvatarRepository) CreateAvatar(ctx context.Context, avatar *domain.Avatar) (string, error) {
	query := `INSERT INTO avatars (user_id, file_name, mime_type, size_bytes, s3_key, upload_status, processing_status)
        	  VALUES ($1, $2, $3, $4, $5, $6, $7)
        	  RETURNING id`

	var id string
	err := r.DB.QueryRow(ctx, query, avatar.UserID, avatar.FileName, avatar.MimeType, avatar.SizeBytes,
		avatar.S3Key, avatar.UploadStatus, avatar.ProcessingStatus).Scan(&id)
	return id, err
}

func (r *AvatarRepository) GetAvatar(ctx context.Context, id string) (*domain.Avatar, error) {
	query := `SELECT id, user_id, file_name, mime_type, size_bytes, s3_key, thumbnail_s3_keys, upload_status, processing_status, created_at, updated_at, deleted_at 
				FROM avatars 
			   WHERE id = $1 AND deleted_at IS NULL`

	avatar := &domain.Avatar{}
	err := r.DB.QueryRow(ctx, query, id).Scan(&avatar.ID, &avatar.UserID, &avatar.FileName, &avatar.MimeType,
		&avatar.SizeBytes, &avatar.S3Key, &avatar.ThumbnailS3Keys,
		&avatar.UploadStatus, &avatar.ProcessingStatus,
		&avatar.CreatedAt, &avatar.UpdatedAt, &avatar.DeletedAt,
	)
	if err != nil {
		return nil, err
	}
	return avatar, nil
}

func (r *AvatarRepository) SoftDeleteAvatar(ctx context.Context, id string, deletedAt *time.Time) error {
	query := `UPDATE avatars SET deleted_at = $1 WHERE id = $2`
	_, err := r.DB.Exec(ctx, query, deletedAt, id)
	return err
}

func (r *AvatarRepository) ListUserAvatars(ctx context.Context, userID string) ([]*domain.Avatar, int, error) {

	// Запрос для получения списка аватаров с пагинацией
	query := `SELECT id, user_id, file_name, mime_type, size_bytes, s3_key, thumbnail_s3_keys, upload_status, processing_status, created_at, updated_at
			   	FROM avatars
				WHERE user_id = $1 AND deleted_at IS NULL
				ORDER BY created_at DESC`

	rows, err := r.DB.Query(ctx, query, userID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	avatars := []*domain.Avatar{}
	for rows.Next() {
		avatar := &domain.Avatar{}
		err := rows.Scan(
			&avatar.ID, &avatar.UserID, &avatar.FileName, &avatar.MimeType,
			&avatar.SizeBytes, &avatar.S3Key, &avatar.ThumbnailS3Keys,
			&avatar.UploadStatus, &avatar.ProcessingStatus,
			&avatar.CreatedAt, &avatar.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		avatars = append(avatars, avatar)
	}

	// Получаем общее количество аватаров для пользователя
	var totalCount int
	countQuery := `SELECT COUNT(*) FROM avatars WHERE user_id = $1 AND deleted_at IS NULL`
	err = r.DB.QueryRow(ctx, countQuery, userID).Scan(&totalCount)
	if err != nil {
		return nil, 0, err
	}

	return avatars, totalCount, nil
}

func (r *AvatarRepository) UpdateAvatar(ctx context.Context, avatar *domain.Avatar) error {
	query := `UPDATE avatars
				SET file_name = $2, mime_type = $3, size_bytes = $4, s3_key = $5, thumbnail_s3_keys = $6, upload_status = $7, processing_status = $8, updated_at = $9
				WHERE id = $1 AND deleted_at IS NULL`

	result, err := r.DB.Exec(ctx, query, avatar.ID, avatar.FileName, avatar.MimeType, avatar.SizeBytes, avatar.S3Key, avatar.ThumbnailS3Keys,
		avatar.UploadStatus, avatar.ProcessingStatus, avatar.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to update avatar %s: %w", avatar.ID, err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("avatar %s not found or already deleted", avatar.ID)
	}

	return nil
}

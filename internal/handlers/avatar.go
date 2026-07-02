package handlers

import (
	"bytes"
	"context"
	"crypto/md5"
	"embed"
	"errors"
	"fmt"
	"go-avatar-service/internal/config"
	"go-avatar-service/internal/domain"
	"go-avatar-service/internal/logger"
	"go-avatar-service/internal/repository"
	"go-avatar-service/internal/services"
	"html/template"
	"image"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type Handler struct {
	Storage   repository.AvatarRepository
	cfg       *config.Config
	serv      *services.S3Service
	queu      *services.QueueService
	logger    logger.Tlog
	templates *template.Template
}

// Константы для валидации
const (
	MaxFileSize = 10 << 20 // 10 MB
	ValidJPEG   = "image/jpeg"
	ValidPNG    = "image/png"
	ValidWebP   = "image/webp"
)

func NewHandler(stor repository.AvatarRepository, cfg *config.Config, serv *services.S3Service, queu *services.QueueService, log logger.Tlog) *Handler {
	return &Handler{
		Storage:   stor,
		cfg:       cfg,
		serv:      serv,
		queu:      queu,
		logger:    log,
		templates: template.Must(loadTemplates()), //template.Must(template.ParseGlob("web/templates/*.html")),
	}
}

func (h *Handler) UploadAvatar(c echo.Context) error {
	// Проверка заголовка X-User-ID
	userID := c.Request().Header.Get("X-User-ID")
	if userID == "" {
		h.logger.Log.Debug("X-User-ID header is required")
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "X-User-ID header is required",
		})
	}

	ctx := c.Request().Context()

	// Получение файла
	file, err := c.FormFile("file")
	if err != nil {
		h.logger.Log.Error("c.FormFile: failed", zap.Error(err))
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "No file provided",
		})
	}

	// Проверка размера файла
	if file.Size > MaxFileSize {
		h.logger.Log.Error("File too large")
		return c.JSON(http.StatusRequestEntityTooLarge, map[string]interface{}{
			"error":    "File too large",
			"max_size": MaxFileSize,
		})
	}

	// Открытие файла для чтения
	src, err := file.Open()
	if err != nil {
		h.logger.Log.Error("failed file.Open:", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to open file",
		})
	}
	defer src.Close()

	// Валидация формата
	buffer := make([]byte, 512)
	_, err = io.ReadFull(src, buffer)
	if err != nil && err != io.ErrUnexpectedEOF {
		h.logger.Log.Error("failed io.ReadFull:", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to read file header",
		})
	}

	detectedType := http.DetectContentType(buffer)
	validTypes := map[string]bool{
		ValidJPEG: true,
		ValidPNG:  true,
		ValidWebP: true,
	}

	if !validTypes[detectedType] {
		h.logger.Log.Error("Invalid file format")
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error":   "Invalid file format",
			"details": "Supported formats: jpeg",
		})
	}

	// Подготовка данных для сохранения
	avatar := &domain.Avatar{
		UserID:           userID,
		FileName:         file.Filename,
		MimeType:         detectedType,
		SizeBytes:        file.Size,
		UploadStatus:     "uploading",
		ProcessingStatus: "pending",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	//  сохранение: сначала S3, затем БД
	s3Key := fmt.Sprintf("avatars/%s/%s", userID, file.Filename)

	err = h.serv.Upload(s3Key, io.MultiReader(bytes.NewReader(buffer), src))
	if err != nil {
		h.logger.Log.Error("failed h.serv.Upload:", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to upload to S3",
		})
	}

	avatar.S3Key = s3Key

	avatarID, err := h.Storage.CreateAvatar(ctx, avatar)
	if err != nil {
		// При ошибке в БД пытаемся удалить файл из S3
		h.logger.Log.Error("h.Storage.CreateAvatar:", zap.Error(err))
		_ = h.serv.Delete(s3Key)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to save metadata",
		})
	}

	avatar.ID = avatarID

	// Отправка события в брокер для обработки
	event := &domain.AvatarUploadEvent{
		AvatarID: avatarID,
		UserID:   userID,
		S3Key:    s3Key,
	}

	err = h.queu.Publish(event)
	if err != nil {
		// Логируем ошибку
		h.logger.Log.Error("Failed to publish event for avatar", zap.String("avatarID", avatarID), zap.Error(err))
	}

	// Возврат ответа
	return c.JSON(http.StatusCreated, map[string]interface{}{
		"id":         avatarID,
		"user_id":    userID,
		"url":        fmt.Sprintf("/api/avatars/%s", avatarID),
		"status":     "processing",
		"created_at": avatar.CreatedAt.UTC().Format(time.RFC3339),
	})
}

func (h *Handler) GetAvatar(c echo.Context) error {
	avatarID := c.Param("user_id")

	// Получаем query‑параметры
	size := c.QueryParam("size")
	format := c.QueryParam("format")

	// Валидация параметров
	validSizes := map[string]bool{"100x100": true, "300x300": true, "original": true}
	validFormats := map[string]bool{"jpeg": true, "png": true, "webp": true}

	if size != "" && !validSizes[size] {
		h.logger.Log.Error("Invalid size parameter")
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error":   "Invalid size parameter",
			"details": "Valid sizes: 100x100, 300x300, original",
		})
	}

	if format != "" && !validFormats[format] {
		h.logger.Log.Error("Invalid format parameter")
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error":   "Invalid format parameter",
			"details": "Valid formats: jpeg, png, webp",
		})
	}

	ctx := c.Request().Context()

	// Получаем метаданные из БД
	avatar, err := h.Storage.GetAvatar(ctx, avatarID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.logger.Log.Error("error: Avatar not found")
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Avatar not found"})
		}
		h.logger.Log.Error("h.Storage.GetAvatar:", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve avatar metadata"})
	}

	// Определяем, какой файл отдавать
	var s3Key string
	var contentType string

	switch {
	case size == "original":
		s3Key = avatar.S3Key
		contentType = avatar.MimeType
	case size != "":
		// Ищем миниатюру нужного размера
		thumbKey, exists := avatar.ThumbnailS3Keys[size]
		if !exists {
			h.logger.Log.Error("error: Thumbnail not found for requested size")
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Thumbnail not found for requested size"})
		}
		s3Key = thumbKey
		// Корректируем content‑type в зависимости от формата
		if format != "" {
			contentType = "image/" + format
		} else {
			contentType = avatar.MimeType // сохраняем оригинальный тип
		}
	default:
		// Если size не указан, отдаём оригинал
		s3Key = avatar.S3Key
		contentType = avatar.MimeType
	}

	// Загружаем файл из S3
	reader, err := h.serv.Download(s3Key)
	if err != nil {
		h.logger.Log.Error("failed h.serv.Download:", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to download file from storage"})
	}
	defer reader.Close()

	// Устанавливаем заголовки
	c.Response().Header().Set("Content-Type", contentType)
	c.Response().Header().Set("Cache-Control", "max-age=86400")
	c.Response().Header().Set("ETag", generateETag(avatarID, size, format))

	// Отдаём бинарные данные изображения
	return c.Stream(http.StatusOK, contentType, reader)
}

// Определяет Content-Type для ответа
func getContentType(format, originalType string) string {
	if format != "" {
		switch format {
		case "jpeg", "jpg":
			return "image/jpeg"
		case "png":
			return "image/png"
		case "webp":
			return "image/webp"
		}
	}
	return originalType
}

func generateETag(avatarID, size, format string) string {
	data := fmt.Sprintf("%s-%s-%s", avatarID, size, format)
	hash := md5.Sum([]byte(data))
	return fmt.Sprintf("%x", hash)
}

func (h *Handler) DeleteAvatar(c echo.Context) error {
	avatarID := c.Param("avatar_id")
	userID := c.Request().Header.Get("X-User-ID")

	if userID == "" {
		h.logger.Log.Debug("X-User-ID header is required")
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "X-User-ID header is required",
		})
	}

	ctx := c.Request().Context()

	// Получаем метаданные аватара из БД
	avatar, err := h.Storage.GetAvatar(ctx, avatarID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.logger.Log.Error("Avatar not found")
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Avatar not found"})
		}
		h.logger.Log.Error("failed h.Storage.GetAvatar: ", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve avatar metadata"})
	}

	// Проверяем, что пользователь пытается удалить свой аватар
	if avatar.UserID != userID {
		h.logger.Log.Error("Forbidden delete avatar")
		return c.JSON(http.StatusForbidden, map[string]string{
			"error":   "Forbidden",
			"details": "You can only delete your own avatars",
		})
	}

	// Мягкое удаление в БД — устанавливаем deleted_at
	deletedAt := time.Now()
	err = h.Storage.SoftDeleteAvatar(ctx, avatarID, &deletedAt)
	if err != nil {
		h.logger.Log.Error("failed h.Storage.SoftDeleteAvatar:", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to mark avatar as deleted"})
	}

	// Подготавливаем событие для асинхронного удаления из S3
	s3KeysToDelete := []string{avatar.S3Key}
	for _, thumbKey := range avatar.ThumbnailS3Keys {
		s3KeysToDelete = append(s3KeysToDelete, thumbKey)
	}

	deleteEvent := &domain.AvatarDeleteEvent{
		AvatarID: avatarID,
		S3Keys:   s3KeysToDelete,
	}

	// Отправляем событие в брокер
	err = h.queu.PublishDeleteEvent(deleteEvent)
	if err != nil {
		h.logger.Log.Error("Warning: failed to publish delete event for avatar", zap.String("avatarID", avatarID), zap.Error(err))
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) GetAvatarMetadata(c echo.Context) error {
	avatarID := c.Param("id")
	ctx := c.Request().Context()

	// Получаем метаданные аватара из БД
	avatar, err := h.Storage.GetAvatar(ctx, avatarID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.logger.Log.Error("Avatar %s not found", zap.String("avatar", avatarID))
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "Avatar not found",
			})
		}
		h.logger.Log.Error("Failed to retrieve avatar metadata")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to retrieve avatar metadata",
		})
	}

	// Формируем ответ в соответствии с требованиями
	response := map[string]interface{}{
		"id":        avatar.ID,
		"user_id":   avatar.UserID,
		"file_name": avatar.FileName,
		"mime_type": avatar.MimeType,
		"size":      avatar.SizeBytes,
		"dimensions": map[string]int{
			"width":  0, // Заглушка — нужно извлекать из изображения
			"height": 0,
		},
		"thumbnails": []map[string]string{},
		"created_at": avatar.CreatedAt.Format(time.RFC3339),
		"updated_at": avatar.UpdatedAt.Format(time.RFC3339),
	}

	// Заполняем размеры изображения (если доступно)
	if dimensions, err := extractImageDimensions(h.serv, avatar.S3Key); err == nil {
		response["dimensions"] = dimensions
	}

	// Заполняем информацию о миниатюрах
	for size := range avatar.ThumbnailS3Keys {
		thumbnail := map[string]string{
			"size": size,
			"url":  fmt.Sprintf("/api/avatars/%s?size=%s", avatar.ID, size),
		}
		response["thumbnails"] = append(response["thumbnails"].([]map[string]string), thumbnail)
	}

	return c.JSON(http.StatusOK, response)
}

func extractImageDimensions(serv *services.S3Service, s3Key string) (map[string]int, error) {
	// Загружаем файл из S3
	reader, err := serv.Download(s3Key)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	img, _, err := image.DecodeConfig(reader)
	if err != nil {
		return nil, err
	}

	return map[string]int{
		"width":  img.Width,
		"height": img.Height,
	}, nil
}

func (h *Handler) ListUserAvatars(c echo.Context) error {
	userID := c.Param("user_id")
	ctx := c.Request().Context()

	// Получаем список аватаров пользователя из БД
	avatars, totalCount, err := h.Storage.ListUserAvatars(ctx, userID)
	if err != nil {
		h.logger.Log.Error("h.Storage.ListUserAvatars", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to retrieve avatars list",
		})
	}

	// Формируем массив ответов
	responseAvatars := make([]map[string]interface{}, 0, len(avatars))
	for _, avatar := range avatars {
		responseAvatar := map[string]interface{}{
			"id":         avatar.ID,
			"file_name":  avatar.FileName,
			"mime_type":  avatar.MimeType,
			"size":       avatar.SizeBytes,
			"status":     avatar.ProcessingStatus,
			"created_at": avatar.CreatedAt.Format(time.RFC3339),
			"url":        fmt.Sprintf("/api/v1/avatars/%s", avatar.ID),
		}

		// Добавляем URL миниатюр, если они есть
		if len(avatar.ThumbnailS3Keys) > 0 {
			thumbnails := make([]map[string]string, 0)
			for size := range avatar.ThumbnailS3Keys {
				thumbnails = append(thumbnails, map[string]string{
					"size": size,
					"url":  fmt.Sprintf("/api/v1/avatars/%s?size=%s", avatar.ID, size),
				})
			}
			responseAvatar["thumbnails"] = thumbnails
		}

		responseAvatars = append(responseAvatars, responseAvatar)
	}

	// Формируем итоговый ответ с метаданными пагинации
	response := map[string]interface{}{
		"user_id": userID,
		"total":   totalCount,
		"avatars": responseAvatars,
	}

	return c.JSON(http.StatusOK, response)
}

func (h *Handler) HealthCheck(c echo.Context) error {
	health := map[string]interface{}{
		"status": "healthy",
		"checks": map[string]string{},
	}

	ctx := c.Request().Context()

	// Проверка БД
	if err := h.Storage.DB.Ping(ctx); err != nil {
		health["status"] = "unhealthy"
		health["database"] = "failed"
	} else {
		health["database"] = "ok"
	}

	// Проверка S3
	if _, err := h.serv.Client.BucketExists(context.Background(), h.serv.Bucket); err != nil {
		health["status"] = "unhealthy"
		health["s3"] = "failed"
	} else {
		health["s3"] = "ok"
	}

	// Проверка очереди
	if !h.queu.IsConnected() {
		health["status"] = "unhealthy"
		health["queue"] = "failed"
	} else {
		health["queue"] = "ok"
	}

	return c.JSON(http.StatusOK, health)
}

// GET /web/upload — отображение формы загрузки
func (h *Handler) UploadForm(c echo.Context) error {
	// Используем встроенный шаблон напрямую
	err := h.templates.ExecuteTemplate(c.Response(), "upload.html", nil)
	if err != nil {
		return err
	}
	c.Response().WriteHeader(http.StatusOK)

	return nil
}

// POST /web/upload — обработка загрузки файла
func (h *Handler) HandleWebUpload(c echo.Context) error {
	// Извлекаем user_id из формы
	userID := c.FormValue("user_id")
	if userID == "" {
		return c.HTML(http.StatusBadRequest, "User ID обязателен")
	}

	// Устанавливаем заголовок для совместимости с API
	c.Request().Header.Set("X-User-ID", userID)

	// Передаём обработку основному обработчику API
	return h.UploadAvatar(c) // функция из API
}

// GET /web/gallery/{user_id} — отображение галереи аватарок
func (h *Handler) Gallery(c echo.Context) error {
	userID := c.Param("user_id")

	// Получаем список аватарок через API (можно напрямую из репозитория)
	avatars, _, err := h.Storage.ListUserAvatars(c.Request().Context(), userID)
	if err != nil {
		return c.HTML(http.StatusInternalServerError, "Ошибка загрузки галереи")
	}

	return c.Render(http.StatusOK, "gallery.html", map[string]interface{}{
		"UserID":  userID,
		"Avatars": avatars,
	})
}

// go:embed web/templates/*.html
var templateFS embed.FS

func loadTemplates() (*template.Template, error) {
	tmpl, err := template.ParseFS(templateFS, "web/templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}
	return tmpl, nil
}

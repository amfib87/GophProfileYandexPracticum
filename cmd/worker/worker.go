package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go-avatar-service/internal/domain"
	"go-avatar-service/internal/repository"
	"image"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go-avatar-service/internal/services"

	"github.com/disintegration/imaging"
)

type Worker struct {
	queue     *services.QueueService
	s3Service *services.S3Service
	repo      *repository.AvatarRepository
	resizer   *ImageResizer
}

func NewWorker(queue *services.QueueService, s3Service *services.S3Service, repo *repository.AvatarRepository, resizer *ImageResizer) *Worker {
	return &Worker{
		queue:     queue,
		s3Service: s3Service,
		repo:      repo,
		resizer:   resizer,
	}
}

func (w *Worker) Start(ctx context.Context) error {
	// Обработчик сигналов ОС для graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Worker received shutdown signal")
		w.queue.Close()
	}()

	// Запуск потребителя сообщений из очереди
	err := w.queue.Consume(func(msg []byte) error {
		return w.processMessage(ctx, msg)
	})
	if err != nil {
		return err
	}
	<-ctx.Done()
	return nil
}

func (w *Worker) processMessage(ctx context.Context, msg []byte) error {
	var event domain.AvatarUploadEvent
	err := json.Unmarshal(msg, &event)
	if err != nil {
		log.Printf("Failed to unmarshal event: %v", err)
		return err
	}

	log.Printf("Processing avatar upload event: %s", event.AvatarID)

	// Идемпотентная обработка — проверяем статус перед выполнением
	avatar, err := w.repo.GetAvatar(ctx, event.AvatarID)
	if err != nil {
		log.Printf("Avatar not found: %s", event.AvatarID)
		return err
	}

	if avatar.ProcessingStatus == "completed" {
		log.Printf("Avatar %s already processed, skipping", event.AvatarID)
		return nil
	}

	// Обработка с retry-механизмом
	err = WithRetry(func() error {
		return w.HandleUploadEvent(ctx, &event)
	}, 3)

	if err != nil {
		log.Printf("Failed to process avatar %s after retries: %v", event.AvatarID, err)
		// Можно отправить в DLQ (Dead Letter Queue) для ручной обработки
		return err
	}

	return nil
}

func WithRetry(fn func() error, maxRetries int) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		// Экспоненциальная задержка
		time.Sleep(time.Duration(1<<i) * time.Second)
	}
	return lastErr
}

func (p *Worker) HandleUploadEvent(ctx context.Context, event *domain.AvatarUploadEvent) error {
	// Получаем метаданные из БД
	avatar, err := p.repo.GetAvatar(ctx, event.AvatarID)
	if err != nil {
		return err
	}

	// Загружаем оригинал из S3
	original, err := p.s3Service.Download(event.S3Key)
	if err != nil {
		return err
	}
	defer original.Close()

	_, _, err = image.Decode(original)
	if err != nil {
		return err
	}

	// Создаём миниатюры
	thumbnails := make(map[string]string)
	sizes := []struct {
		size   string
		width  int
		height int
	}{
		{"100x100", 100, 100},
		{"300x300", 300, 300},
	}

	w := NewImageResizer()

	for _, s := range sizes {
		// JPEG версия
		jpegData, err := w.Resize(original, s.width, s.height, imaging.JPEG)
		if err != nil {
			return err
		}
		jpegKey := fmt.Sprintf("thumbnails/%s/%s.jpg", event.AvatarID, s.size)
		err = p.s3Service.Upload(jpegKey, bytes.NewReader(jpegData))
		if err != nil {
			return err
		}
		thumbnails[s.size+"_jpeg"] = jpegKey
	}

	// Обновляем метаданные в БД
	avatar.ThumbnailS3Keys = thumbnails
	avatar.ProcessingStatus = "completed"
	avatar.UpdatedAt = time.Now()

	err = p.repo.UpdateAvatar(ctx, avatar)
	if err != nil {
		return err
	}

	log.Printf("Successfully processed avatar %s", event.AvatarID)
	return nil
}

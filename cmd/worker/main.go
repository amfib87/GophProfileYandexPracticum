package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"go-avatar-service/internal/config"
	"go-avatar-service/internal/logger"
	"go-avatar-service/internal/repository"
	"go-avatar-service/internal/services"

	"go.uber.org/zap"
)

func main() {

	if err := run(); err != nil {
		log.Fatal("runtime error:", err)
	}
}

func run() error {

	// Создаем логгер
	logger, err := logger.Initialization()
	if err != nil {
		return err
	}
	logger.Log.Debug("logger was successfulle created")

	cfg := config.NewConfig()
	cfg.GetEnvData()
	logger.Log.Debug("flags were parsed", zap.Any("cfg", cfg))

	// Инициализируем БД
	db, err := repository.Initialization(cfg.PostgresURL)
	if err != nil {
		return err
	}

	// Инициализация S3‑сервиса для worker’а
	s3Service, err := services.NewS3Service(cfg.MinIOEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey, cfg.MinIOBucket)
	if err != nil {
		return err
	}

	// Инициализируем брокер
	rabbitMQ, err := services.NewRabbitMQService(cfg.RabbitMQURL, "avatar-processing")
	if err != nil {
		log.Fatalf("Failed to initialize RabbitMQ: %v", err)
	}

	// Создание сервиса ресайза изображений
	resizer := NewImageResizer()

	// Создание воркера
	worker := NewWorker(rabbitMQ, s3Service, &db, resizer)

	// Создаём контекст, который отменится при SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Запуск воркера
	log.Println("Starting avatar processing worker...")
	err = worker.Start(ctx)
	if err != nil {
		log.Printf("Worker stopped with error: %v", err)
		os.Exit(1)
	}

	log.Println("Worker shutdown completed gracefully")
	return nil
}

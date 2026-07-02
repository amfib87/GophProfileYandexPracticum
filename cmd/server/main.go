package main

import (
	"go-avatar-service/internal/config"
	"go-avatar-service/internal/handlers"
	"go-avatar-service/internal/logger"
	"go-avatar-service/internal/repository"
	"go-avatar-service/internal/router"
	"go-avatar-service/internal/services"
	"log"
	"net/http"

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

	// Инициализируем БД
	db, err := repository.Initialization(cfg.PostgresURL)
	if err != nil {
		logger.Log.Error("failed repository.Initialization %v", zap.Error(err))
		return err
	}

	// Инициализация S3‑сервиса для worker’а
	s3Service, err := services.NewS3Service(cfg.MinIOEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey, cfg.MinIOBucket)
	if err != nil {
		logger.Log.Error("services.NewS3Service: %v", zap.Error(err))
		return err
	}

	// Инициализируем брокер
	rabbitMQ, err := services.NewRabbitMQService(cfg.RabbitMQURL, "avatar-processing")
	if err != nil {
		logger.Log.Error("Failed to initialize RabbitMQ: %v", zap.Error(err))
		return err
	}

	// Инициализируем хэндлер
	handler := handlers.NewHandler(db, cfg, s3Service, rabbitMQ, logger)

	// Инициализируем роутер
	router := router.Initialization(handler)

	logger.Log.Info("running server", zap.String("cfg.RunAddress)", cfg.ServRunAddr))
	if err := http.ListenAndServe(cfg.ServRunAddr, router); err != nil {
		logger.Log.Error("failed http.ListenAndServe: %v", zap.Error(err))
		return err
	}

	return nil
}

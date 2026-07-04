package router

import (
	"go-avatar-service/internal/handlers"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type Router struct {
	Echo *echo.Echo
}

func Initialization(h *handlers.Handler) *Router {
	e := echo.New()

	// Добавляем  ендпоинт для метрик
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))

	// Эндпоинты API
	e.POST("/api/v1/avatars", withOtel(h.UploadAvatar, "UploadAvatar")) // # Загрузка аватарки

	e.GET("/api/v1/avatars/:avatar_id", withOtel(h.GetAvatar, "GetAvatar"))    // # Получение аватарки
	e.GET("/api/v1/users/:user_id/avatar", withOtel(h.GetAvatar, "GetAvatar")) // # Получение аватарки

	e.DELETE("/api/v1/avatars/:avatar_id", withOtel(h.DeleteAvatar, "DeleteAvatar"))    // # Удаление аватарки
	e.DELETE("/api/v1/users/:user_id/avatar", withOtel(h.DeleteAvatar, "DeleteAvatar")) // # Удаление аватарки

	e.GET("/api/v1/avatars/:id/metadata", withOtel(h.GetAvatarMetadata, "GetAvatarMetadata")) // # Получение метаданных аватарки

	e.GET("/api/v1/users/:user_id/avatars", withOtel(h.ListUserAvatars, "ListUserAvatars")) // # Список аватарок пользователя

	e.GET("/health", withOtel(h.HealthCheck, "HealthCheck")) // # Проверка работоспособности

	e.GET("/web/upload", withOtel(h.UploadForm, "UploadForm"))            // форма загрузки
	e.POST("/web/upload", withOtel(h.HandleWebUpload, "HandleWebUpload")) // обработка загрузки
	e.GET("/web/gallery/:user_id", withOtel(h.Gallery, "Gallery"))        // галерея аватарок

	return &Router{Echo: e}
}

// ServeHTTP реализует интерфейс http.Handler
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.Echo.ServeHTTP(w, req)
}

// withOtel - поыптка интегрировать функционал otephhtp с echo.
// withOtel оборачивает echo.HandlerFunc для работы с otelhttp, сохраняя параметры пути.
func withOtel(handler echo.HandlerFunc, operationName string) echo.HandlerFunc {
	return func(c echo.Context) error {
		w := c.Response().Writer
		r := c.Request()

		httpH := http.HandlerFunc(func(httpW http.ResponseWriter, httpR *http.Request) {
			handler(c)
		})

		otelH := otelhttp.NewHandler(httpH, operationName)
		otelH.ServeHTTP(w, r)

		return nil
	}
}

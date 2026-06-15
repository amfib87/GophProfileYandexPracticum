package router

import (
	"go-avatar-service/internal/handlers"
	"net/http"

	"github.com/labstack/echo/v4"
)

type Router struct {
	Echo *echo.Echo
}

func Initialization(h *handlers.Handler) *Router {
	e := echo.New()

	// Эндпоинты API
	e.POST("/api/v1/avatars", h.UploadAvatar) // # Загрузка аватарки

	e.GET("/api/v1/avatars/:avatar_id", h.GetAvatar)    // # Получение аватарки
	e.GET("/api/v1/users/:user_id/avatar", h.GetAvatar) // # Получение аватарки

	e.DELETE("/api/v1/avatars/:avatar_id", h.DeleteAvatar)    // # Удаление аватарки
	e.DELETE("/api/v1/users/:user_id/avatar", h.DeleteAvatar) // # Удаление аватарки

	e.GET("/api/v1/avatars/:id/metadata", h.GetAvatarMetadata) // # Получение метаданных аватарки

	e.GET("/api/v1/users/:user_id/avatar", h.ListUserAvatars) // # Список аватарок пользователя

	e.GET("/health", h.HealthCheck) // # Проверка работоспособности

	e.GET("/web/upload", h.UploadForm)        // форма загрузки
	e.POST("/web/upload", h.HandleWebUpload)  // обработка загрузки
	e.GET("/web/gallery/:user_id", h.Gallery) // галерея аватарок

	return &Router{Echo: e}
}

// ServeHTTP реализует интерфейс http.Handler
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.Echo.ServeHTTP(w, req)
}

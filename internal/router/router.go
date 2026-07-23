package router

import (
	"context"
	"fmt"
	"go-avatar-service/internal/handlers"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
	"go.opentelemetry.io/otel/trace"
)

type Router struct {
	Echo *echo.Echo
}

func Initialization(h *handlers.Handler) *Router {
	e := echo.New()

	// Добавляем  ендпоинт для метрик
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))

	// Регистрируем middleware глобально
	e.Use(OtelMiddleware())

	// Эндпоинты API
	e.POST("/api/v1/avatars", h.UploadAvatar) // # Загрузка аватарки

	e.GET("/api/v1/avatars/:avatar_id", h.GetAvatar)    // # Получение аватарки
	e.GET("/api/v1/users/:user_id/avatar", h.GetAvatar) // # Получение аватарки

	e.DELETE("/api/v1/avatars/:avatar_id", h.DeleteAvatar)    // # Удаление аватарки
	e.DELETE("/api/v1/users/:user_id/avatar", h.DeleteAvatar) // # Удаление аватарки

	e.GET("/api/v1/avatars/:id/metadata", h.GetAvatarMetadata) // # Получение метаданных аватарки

	e.GET("/api/v1/users/:user_id/avatars", h.ListUserAvatars) // # Список аватарок пользователя

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

func OtelMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx, span := startSpan(c)
			defer span.End()

			// Обновляем контекст запроса, чтобы сервисы ниже могли брать из него спан
			req := c.Request().WithContext(ctx)
			c.SetRequest(req)

			if err := next(c); err != nil {
				// Если хендлер вернул ошибку, помечаем span как ошибочный
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())

				// Возвращаем ошибку дальше в Echo
				return err
			}

			return nil
		}
	}
}

func startSpan(c echo.Context) (context.Context, trace.Span) {
	tracer := otel.Tracer("GophProfile")

	// Получаем шаблон пути из Echo
	route := c.Path()
	method := c.Request().Method

	operationName := fmt.Sprintf("%s %s", method, route)

	ctx, span := tracer.Start(c.Request().Context(), operationName)

	span.SetAttributes(
		semconv.HTTPMethodKey.String(method),
		semconv.HTTPTargetKey.String(c.Path()),
		semconv.HTTPRouteKey.String(route),
	)

	return ctx, span
}

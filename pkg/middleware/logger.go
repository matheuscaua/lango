package middleware

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// RequestLogger injects a unique request_id into each request, sets it as a
// response header, and logs entry/exit with method, path, status and duration.
func RequestLogger() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		requestID := uuid.Must(uuid.NewV7()).String()
		c.Set("X-Request-ID", requestID)
		c.Locals("request_id", requestID)

		err := c.Next()

		duration := time.Since(start)
		status := c.Response().StatusCode()

		attrs := []slog.Attr{
			slog.String("request_id", requestID),
			slog.String("method", c.Method()),
			slog.String("path", c.Path()),
			slog.Int("status", status),
			slog.Duration("latency", duration),
		}

		if consumerID, ok := c.Locals(localsKeyConsumerID).(uuid.UUID); ok {
			attrs = append(attrs, slog.String("consumer_id", consumerID.String()))
		}

		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		} else if status >= 400 {
			level = slog.LevelWarn
		}

		slog.LogAttrs(c.Context(), level, "http", attrs...)
		return err
	}
}

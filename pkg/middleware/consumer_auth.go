// Package middleware provides Fiber HTTP middleware for consumer
// authentication and request logging.
package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/kituomenyu/lango/internal/domain"
	pkgauth "github.com/kituomenyu/lango/pkg/auth"
)

const localsKeyConsumerID = "consumer_id"

// ConsumerAuth resolves the X-Lango-Api-Key header to a Consumer and stores
// its ID in Fiber Locals. Every consumer gets its own key — never a shared
// global secret — so lango always knows exactly who is asking (ADR 008,
// "Identificação de consumidores").
func ConsumerAuth(consumers domain.ConsumerRepository) fiber.Handler {
	return func(c *fiber.Ctx) error {
		apiKey := c.Get("X-Lango-Api-Key")
		if apiKey == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "missing X-Lango-Api-Key header")
		}

		consumer, err := consumers.GetByAPIKeyHash(c.Context(), pkgauth.HashAPIKey(apiKey))
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid api key")
		}
		if !consumer.Active {
			return fiber.NewError(fiber.StatusForbidden, "consumer is inactive")
		}

		c.Locals(localsKeyConsumerID, consumer.ID)
		return c.Next()
	}
}

// ConsumerIDFromLocals retrieves the consumer UUID stored by ConsumerAuth.
func ConsumerIDFromLocals(c *fiber.Ctx) (uuid.UUID, bool) {
	id, ok := c.Locals(localsKeyConsumerID).(uuid.UUID)
	return id, ok
}

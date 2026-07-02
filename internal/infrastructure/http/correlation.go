package http

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const correlationIDHeader = "X-Correlation-Id"

// correlationIDFrom returns the caller-supplied X-Correlation-Id, generating
// a fresh one if absent — extends the trace_id convention from haraka's
// ADR-004 across the lango leg of the chain (ADR 008, "Assertividade").
func correlationIDFrom(c *fiber.Ctx) string {
	if id := c.Get(correlationIDHeader); id != "" {
		return id
	}
	return uuid.Must(uuid.NewV7()).String()
}

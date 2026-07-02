package http

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/kituomenyu/lango/internal/domain"
	pkgmiddleware "github.com/kituomenyu/lango/pkg/middleware"
)

// AuditHandler exposes read access to a consumer's own message audit trail —
// the "consultas" required by ADR 008, and the tool for spotting a consumer
// sending outside its normal pattern.
type AuditHandler struct {
	repo domain.MessageAuditRepository
}

func NewAuditHandler(repo domain.MessageAuditRepository) *AuditHandler {
	return &AuditHandler{repo: repo}
}

// List handles GET /v1/audit?integration_id=&status=&limit=
func (h *AuditHandler) List(c *fiber.Ctx) error {
	consumerID, ok := pkgmiddleware.ConsumerIDFromLocals(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "missing consumer context")
	}

	var integrationFilter *uuid.UUID
	if raw := c.Query("integration_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid integration_id")
		}
		integrationFilter = &id
	}

	limit := 100
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}

	entries, err := h.repo.ListByConsumer(c.Context(), consumerID, integrationFilter, domain.AuditStatus(c.Query("status")), limit)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to list audit entries")
	}

	out := make([]fiber.Map, 0, len(entries))
	for _, e := range entries {
		out = append(out, fiber.Map{
			"id":             e.ID,
			"integration_id": e.IntegrationID,
			"direction":      e.Direction,
			"provider":       e.Provider,
			"to_number":      e.ToNumber,
			"from_number":    e.FromNumber,
			"external_id":    e.ExternalID,
			"status":         e.Status,
			"error_reason":   e.ErrorReason,
			"correlation_id": e.CorrelationID,
			"created_at":     e.CreatedAt,
		})
	}
	return c.JSON(fiber.Map{"entries": out})
}

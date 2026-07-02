package http

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/kituomenyu/lango/internal/domain"
	pkgmiddleware "github.com/kituomenyu/lango/pkg/middleware"
)

// IntegrationHandler exposes CRUD for WhatsApp integrations, always scoped to
// the authenticated consumer — a consumer can never read or modify an
// integration it doesn't own (ADR 008).
type IntegrationHandler struct {
	repo domain.IntegrationRepository
}

func NewIntegrationHandler(repo domain.IntegrationRepository) *IntegrationHandler {
	return &IntegrationHandler{repo: repo}
}

type createIntegrationRequest struct {
	Provider      string `json:"provider"`
	PhoneNumberID string `json:"phone_number_id"`
	AccessToken   string `json:"access_token"`
	VerifyToken   string `json:"verify_token"`
}

// Create handles POST /v1/integrations
func (h *IntegrationHandler) Create(c *fiber.Ctx) error {
	consumerID, ok := pkgmiddleware.ConsumerIDFromLocals(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "missing consumer context")
	}

	var req createIntegrationRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.Provider != "meta" && req.Provider != "evolution" && req.Provider != "twilio" {
		return fiber.NewError(fiber.StatusBadRequest, "provider must be one of: meta, evolution, twilio")
	}
	if req.PhoneNumberID == "" || req.AccessToken == "" {
		return fiber.NewError(fiber.StatusBadRequest, "phone_number_id and access_token are required")
	}

	integration := &domain.Integration{
		ConsumerID:    consumerID,
		Provider:      req.Provider,
		PhoneNumberID: req.PhoneNumberID,
		AccessToken:   req.AccessToken,
		VerifyToken:   req.VerifyToken,
		Active:        true,
	}
	if err := h.repo.Save(c.Context(), integration); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to save integration")
	}

	return c.Status(fiber.StatusCreated).JSON(integrationResponse(integration))
}

// List handles GET /v1/integrations
func (h *IntegrationHandler) List(c *fiber.Ctx) error {
	consumerID, ok := pkgmiddleware.ConsumerIDFromLocals(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "missing consumer context")
	}

	integrations, err := h.repo.ListByConsumer(c.Context(), consumerID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to list integrations")
	}

	out := make([]fiber.Map, 0, len(integrations))
	for _, i := range integrations {
		out = append(out, integrationResponse(i))
	}
	return c.JSON(fiber.Map{"integrations": out})
}

// Get handles GET /v1/integrations/:id
func (h *IntegrationHandler) Get(c *fiber.Ctx) error {
	consumerID, ok := pkgmiddleware.ConsumerIDFromLocals(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "missing consumer context")
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid integration id")
	}

	integration, err := h.repo.GetByID(c.Context(), id)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "integration not found")
	}
	if integration.ConsumerID != consumerID {
		// Same response as "not found" — never reveal that an integration
		// exists under another consumer.
		return fiber.NewError(fiber.StatusNotFound, "integration not found")
	}

	return c.JSON(integrationResponse(integration))
}

func integrationResponse(i *domain.Integration) fiber.Map {
	return fiber.Map{
		"id":              i.ID,
		"provider":        i.Provider,
		"phone_number_id": i.PhoneNumberID,
		"active":          i.Active,
		"created_at":      i.CreatedAt,
		// access_token and verify_token omitted — never returned
	}
}

package http

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/kituomenyu/lango/internal/application"
	"github.com/kituomenyu/lango/internal/domain"
	pkgmiddleware "github.com/kituomenyu/lango/pkg/middleware"
)

// IntegrationHandler exposes CRUD for WhatsApp integrations, always scoped to
// the authenticated consumer — a consumer can never read or modify an
// integration it doesn't own (ADR 008).
type IntegrationHandler struct {
	repo            domain.IntegrationRepository
	audit           domain.MessageAuditRepository
	evolutionAPIKey string
	connect         *application.ConnectIntegrationUseCase    // nil disables POST /:id/connect (501)
	disconnect      *application.DisconnectIntegrationUseCase // nil disables DELETE /:id (501)
}

func NewIntegrationHandler(
	repo domain.IntegrationRepository,
	audit domain.MessageAuditRepository,
	evolutionAPIKey string,
	connect *application.ConnectIntegrationUseCase,
	disconnect *application.DisconnectIntegrationUseCase,
) *IntegrationHandler {
	return &IntegrationHandler{repo: repo, audit: audit, evolutionAPIKey: evolutionAPIKey, connect: connect, disconnect: disconnect}
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

	integration := &domain.Integration{
		ConsumerID:    consumerID,
		Provider:      req.Provider,
		PhoneNumberID: req.PhoneNumberID,
		AccessToken:   req.AccessToken,
		VerifyToken:   req.VerifyToken,
		Active:        true,
	}

	if req.Provider == "evolution" {
		// Evolution has no per-caller credentials to supply up front: one
		// global admin key (shared by every evolution integration on this
		// lango instance) authenticates instance management, and the
		// instance itself doesn't exist yet — it's created by the connect
		// flow, keyed by this integration's own ID. Not connected until a
		// human scans the QR (see ConnectIntegrationUseCase), so Active
		// starts false.
		id := uuid.New()
		integration.ID = id
		integration.PhoneNumberID = id.String()
		integration.AccessToken = h.evolutionAPIKey
		integration.Active = false
	} else if req.PhoneNumberID == "" || req.AccessToken == "" {
		return fiber.NewError(fiber.StatusBadRequest, "phone_number_id and access_token are required")
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

// Status handles GET /v1/integrations/:id/status — the channel-health summary
// consumers poll to answer "is my WhatsApp number alive?" without access to
// the raw audit trail of other tenants. Counters cover the last 24h.
func (h *IntegrationHandler) Status(c *fiber.Ctx) error {
	consumerID, ok := pkgmiddleware.ConsumerIDFromLocals(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "missing consumer context")
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid integration id")
	}

	integration, err := h.repo.GetByID(c.Context(), id)
	if err != nil || integration.ConsumerID != consumerID {
		// Same response for both cases — never reveal foreign integrations.
		return fiber.NewError(fiber.StatusNotFound, "integration not found")
	}

	summary, err := h.audit.SummarizeIntegration(c.Context(), id, time.Now().UTC().Add(-24*time.Hour))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to summarize integration activity")
	}

	return c.JSON(fiber.Map{
		"integration_id":       integration.ID,
		"provider":             integration.Provider,
		"active":               integration.Active,
		"last_inbound_at":      summary.LastInboundAt,
		"last_outbound_at":     summary.LastOutboundAt,
		"last_outbound_status": string(summary.LastOutboundStatus),
		"sent_24h":             summary.SentCount,
		"failed_24h":           summary.FailedCount,
	})
}

// Connect handles POST /v1/integrations/:id/connect — creates the Evolution
// instance if needed, (re)configures its webhook, and returns either a fresh
// QR code to scan or "connected" if the number is already linked. Callers
// poll this repeatedly until connected (see ConnectIntegrationUseCase).
func (h *IntegrationHandler) Connect(c *fiber.Ctx) error {
	if h.connect == nil {
		return fiber.NewError(fiber.StatusNotImplemented, "evolution connect flow is not configured on this lango instance")
	}

	consumerID, ok := pkgmiddleware.ConsumerIDFromLocals(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "missing consumer context")
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid integration id")
	}

	result, err := h.connect.Execute(c.Context(), application.ConnectIntegrationInput{
		ConsumerID:    consumerID,
		IntegrationID: id,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrIntegrationNotFound), errors.Is(err, domain.ErrIntegrationNotOwned):
			return fiber.NewError(fiber.StatusNotFound, "integration not found")
		case errors.Is(err, application.ErrConnectUnsupportedProvider):
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		default:
			return fiber.NewError(fiber.StatusBadGateway, "failed to connect integration")
		}
	}

	return c.JSON(fiber.Map{
		"state":     string(result.State),
		"qr_base64": result.QRBase64,
	})
}

// Disconnect handles DELETE /v1/integrations/:id — ends the WhatsApp session
// (Evolution logout + instance delete) and removes the integration, so an old
// linked number is actually disconnected and the slot is free for a fresh
// connect. Scoped to the authenticated consumer.
func (h *IntegrationHandler) Disconnect(c *fiber.Ctx) error {
	if h.disconnect == nil {
		return fiber.NewError(fiber.StatusNotImplemented, "disconnect flow is not configured on this lango instance")
	}

	consumerID, ok := pkgmiddleware.ConsumerIDFromLocals(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "missing consumer context")
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid integration id")
	}

	err = h.disconnect.Execute(c.Context(), application.DisconnectIntegrationInput{
		ConsumerID:    consumerID,
		IntegrationID: id,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrIntegrationNotFound), errors.Is(err, domain.ErrIntegrationNotOwned):
			// Already gone (or never owned) → the desired end state is reached.
			return c.SendStatus(fiber.StatusNoContent)
		default:
			return fiber.NewError(fiber.StatusBadGateway, "failed to disconnect integration")
		}
	}

	return c.SendStatus(fiber.StatusNoContent)
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

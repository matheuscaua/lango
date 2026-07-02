package http

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/kituomenyu/lango/internal/application"
	"github.com/kituomenyu/lango/internal/domain"
)

// WebhookHandler handles inbound HTTP events from the WhatsApp Cloud API (Meta).
type WebhookHandler struct {
	integrations domain.IntegrationRepository
	forward      *application.ForwardInboundUseCase
	appSecret    string // Meta App Secret for HMAC-SHA256 signature validation (app-level, shared across integrations)
}

func NewWebhookHandler(integrations domain.IntegrationRepository, forward *application.ForwardInboundUseCase, appSecret string) *WebhookHandler {
	return &WebhookHandler{integrations: integrations, forward: forward, appSecret: appSecret}
}

// VerifyWebhook handles the GET challenge Meta sends when registering a webhook URL.
// Route: GET /webhooks/meta/:integration_id
func (h *WebhookHandler) VerifyWebhook(c *fiber.Ctx) error {
	integrationID, err := uuid.Parse(c.Params("integration_id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid integration_id")
	}

	mode := c.Query("hub.mode")
	token := c.Query("hub.verify_token")
	challenge := c.Query("hub.challenge")

	if mode != "subscribe" || challenge == "" {
		return fiber.NewError(fiber.StatusBadRequest, "missing hub parameters")
	}

	cfg, err := h.integrations.GetByID(c.Context(), integrationID)
	if err != nil {
		slog.WarnContext(c.Context(), "webhook verification: integration not found",
			slog.String("integration_id", integrationID.String()))
		return fiber.NewError(fiber.StatusForbidden, "integration not configured")
	}

	if cfg.VerifyToken != token {
		slog.WarnContext(c.Context(), "webhook verification: token mismatch",
			slog.String("integration_id", integrationID.String()))
		return fiber.NewError(fiber.StatusForbidden, "invalid verify_token")
	}

	return c.SendString(challenge)
}

// ReceiveWebhook handles POST events (inbound messages and delivery status updates).
// Route: POST /webhooks/meta/:integration_id
func (h *WebhookHandler) ReceiveWebhook(c *fiber.Ctx) error {
	integrationID, err := uuid.Parse(c.Params("integration_id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid integration_id")
	}

	if h.appSecret != "" {
		sig := c.Get("X-Hub-Signature-256")
		if !h.verifySignature(c.Body(), sig) {
			slog.WarnContext(c.Context(), "webhook signature validation failed",
				slog.String("integration_id", integrationID.String()))
			return fiber.NewError(fiber.StatusUnauthorized, "invalid webhook signature")
		}
	}

	var payload whatsAppWebhookPayload
	if err := c.BodyParser(&payload); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "malformed webhook payload")
	}

	correlationID := correlationIDFrom(c)
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			if change.Field != "messages" {
				continue
			}
			for _, msg := range change.Value.Messages {
				content := msg.Text.Body
				if msg.Type != "text" {
					content = fmt.Sprintf("[MEDIA:%s]", strings.ToUpper(msg.Type))
				}
				event := domain.InboundEvent{
					From:       msg.From,
					Content:    content,
					ExternalID: msg.ID,
					ReceivedAt: time.Now().UTC(),
				}
				h.forward.Execute(c.Context(), integrationID, event, correlationID)
			}
		}
	}

	return c.SendStatus(fiber.StatusOK)
}

func (h *WebhookHandler) verifySignature(body []byte, signature string) bool {
	const prefix = "sha256="
	if len(signature) <= len(prefix) {
		return false
	}
	mac := hmac.New(sha256.New, []byte(h.appSecret))
	mac.Write(body)
	expected := prefix + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// ── WhatsApp Cloud API payload structures ─────────────────────────────────────

type whatsAppWebhookPayload struct {
	Object string  `json:"object"`
	Entry  []entry `json:"entry"`
}

type entry struct {
	ID      string   `json:"id"`
	Changes []change `json:"changes"`
}

type change struct {
	Field string      `json:"field"`
	Value changeValue `json:"value"`
}

type changeValue struct {
	MessagingProduct string      `json:"messaging_product"`
	Metadata         waMetadata  `json:"metadata"`
	Messages         []waMessage `json:"messages"`
}

type waMetadata struct {
	DisplayPhoneNumber string `json:"display_phone_number"`
	PhoneNumberID      string `json:"phone_number_id"`
}

type waMessage struct {
	From      string `json:"from"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Text      waText `json:"text"`
}

type waText struct {
	Body string `json:"body"`
}

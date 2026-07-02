package http

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/kituomenyu/lango/internal/application"
	"github.com/kituomenyu/lango/internal/domain"
)

// WebhookEvolutionHandler handles inbound HTTP events from a self-hosted Evolution API instance.
type WebhookEvolutionHandler struct {
	forward *application.ForwardInboundUseCase
}

func NewWebhookEvolutionHandler(forward *application.ForwardInboundUseCase) *WebhookEvolutionHandler {
	return &WebhookEvolutionHandler{forward: forward}
}

// ReceiveWebhook handles POST /webhooks/evolution/:integration_id.
// Evolution API does not send a verification challenge — just POST events.
func (h *WebhookEvolutionHandler) ReceiveWebhook(c *fiber.Ctx) error {
	integrationID, err := uuid.Parse(c.Params("integration_id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid integration_id")
	}

	var payload evolutionWebhookPayload
	if err := c.BodyParser(&payload); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "malformed evolution webhook payload")
	}

	if payload.Event != "messages.upsert" {
		return c.SendStatus(fiber.StatusOK)
	}

	data := payload.Data

	// Skip messages sent by us (fromMe=true) or without a remoteJid.
	if data.Key.FromMe || data.Key.RemoteJid == "" {
		return c.SendStatus(fiber.StatusOK)
	}

	// Skip group chats, broadcast lists, status updates and newsletters — a
	// consumer's bot only engages 1:1 customer conversations.
	jid := data.Key.RemoteJid
	if strings.HasSuffix(jid, "@g.us") || strings.HasSuffix(jid, "@broadcast") || strings.HasSuffix(jid, "@newsletter") {
		return c.SendStatus(fiber.StatusOK)
	}

	phone := strings.TrimSuffix(jid, "@s.whatsapp.net")
	phone = strings.TrimSuffix(phone, "@c.us")

	content := extractContent(data)
	if content == "" {
		return c.SendStatus(fiber.StatusOK)
	}

	event := domain.InboundEvent{
		From:       phone,
		Content:    content,
		ExternalID: data.Key.ID,
		ReceivedAt: time.Now().UTC(),
	}
	h.forward.Execute(c.Context(), integrationID, event, correlationIDFrom(c))

	return c.SendStatus(fiber.StatusOK)
}

func extractContent(data evolutionData) string {
	switch data.MessageType {
	case "conversation":
		return data.Message.Conversation
	case "extendedTextMessage":
		return data.Message.ExtendedTextMessage.Text
	case "listResponseMessage":
		// Customer tapped a row in a WhatsApp List Message — the row ID is
		// opaque to lango (see domain.ListRow) and flows through unchanged
		// as ButtonPayload for the consumer to interpret.
		return data.Message.ListResponseMessage.SingleSelectReply.SelectedRowID
	default:
		if data.MessageType != "" {
			return fmt.Sprintf("[MEDIA:%s]", strings.ToUpper(data.MessageType))
		}
		return ""
	}
}

// ── Evolution API webhook payload structures ──────────────────────────────────

type evolutionWebhookPayload struct {
	Event    string        `json:"event"`
	Instance string        `json:"instance"`
	Data     evolutionData `json:"data"`
}

type evolutionData struct {
	Key         evolutionKey     `json:"key"`
	MessageType string           `json:"messageType"`
	Message     evolutionMessage `json:"message"`
}

type evolutionKey struct {
	RemoteJid string `json:"remoteJid"`
	FromMe    bool   `json:"fromMe"`
	ID        string `json:"id"`
}

type evolutionMessage struct {
	Conversation        string                       `json:"conversation"`
	ExtendedTextMessage evolutionExtendedTextMessage `json:"extendedTextMessage"`
	ListResponseMessage evolutionListResponseMessage `json:"listResponseMessage"`
}

type evolutionExtendedTextMessage struct {
	Text string `json:"text"`
}

type evolutionListResponseMessage struct {
	SingleSelectReply evolutionSingleSelectReply `json:"singleSelectReply"`
}

type evolutionSingleSelectReply struct {
	SelectedRowID string `json:"selectedRowId"`
}

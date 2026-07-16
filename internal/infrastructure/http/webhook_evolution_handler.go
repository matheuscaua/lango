package http

import (
	"fmt"
	"log/slog"
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
	audit   domain.MessageAuditRepository // nil disables delivery/read receipt handling
}

func NewWebhookEvolutionHandler(forward *application.ForwardInboundUseCase, audit domain.MessageAuditRepository) *WebhookEvolutionHandler {
	return &WebhookEvolutionHandler{forward: forward, audit: audit}
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

	// Delivery/read acks for messages WE sent arrive as messages.update — record
	// them against the outbound audit entry so the channel status reflects "read".
	eventLower := strings.ToLower(payload.Event)
	eventLower = strings.ReplaceAll(eventLower, "_", ".")

	if eventLower == "messages.update" {
		h.handleStatusUpdate(c, integrationID, payload.Data)
		return c.SendStatus(fiber.StatusOK)
	}

	if eventLower != "messages.upsert" {
		return c.SendStatus(fiber.StatusOK)
	}

	data := payload.Data

	// Support both Evolution API v1 (flat) and v2 (nested inside data.message)
	key := data.Key
	if key.RemoteJid == "" && data.Message.Key.RemoteJid != "" {
		key = data.Message.Key
	}

	// Skip messages sent by us (fromMe=true) or without a remoteJid.
	if key.FromMe || key.RemoteJid == "" {
		return c.SendStatus(fiber.StatusOK)
	}

	phone, ok := resolveInboundPhone(key)
	if !ok {
		return c.SendStatus(fiber.StatusOK)
	}

	content := extractContent(data)
	if content == "" {
		return c.SendStatus(fiber.StatusOK)
	}

	event := domain.InboundEvent{
		From:       phone,
		Content:    content,
		ExternalID: key.ID,
		ReceivedAt: time.Now().UTC(),
	}
	h.forward.Execute(c.Context(), integrationID, event, correlationIDFrom(c))

	return c.SendStatus(fiber.StatusOK)
}

// resolveInboundPhone returns the customer's phone (bare digits, no domain
// suffix) for a 1:1 message, or ok=false when the event must be skipped:
//   - group / broadcast / newsletter (a bot only engages 1:1 conversations)
//   - a LID-addressed message whose real number (remoteJidAlt) is missing,
//     since replying to an "<lid>@lid" JID never reaches the customer.
//
// WhatsApp LID addressing (privacy-preserving identity, increasingly the
// default) puts an opaque Linked ID in remoteJid and the real phone in
// remoteJidAlt — this is where that gets unwrapped.
func resolveInboundPhone(key evolutionKey) (string, bool) {
	jid := key.RemoteJid
	if strings.HasSuffix(jid, "@g.us") || strings.HasSuffix(jid, "@broadcast") || strings.HasSuffix(jid, "@newsletter") {
		return "", false
	}

	if key.AddressingMode == "lid" || strings.HasSuffix(jid, "@lid") {
		if key.RemoteJidAlt == "" {
			return "", false
		}
		jid = key.RemoteJidAlt
	}

	phone := strings.TrimSuffix(jid, "@s.whatsapp.net")
	phone = strings.TrimSuffix(phone, "@c.us")
	return phone, phone != ""
}

// handleStatusUpdate maps an Evolution messages.update ack to an outbound
// delivery status and records it against the matching audit entry. Best-effort:
// unknown statuses, a missing message id, or no audit repo are silently skipped
// (the rank guard in MarkOutboundStatusByExternalID also ignores regressions).
func (h *WebhookEvolutionHandler) handleStatusUpdate(c *fiber.Ctx, integrationID uuid.UUID, data evolutionData) {
	if h.audit == nil {
		return
	}
	// v2 puts the sent message's id in `keyId`; older payloads nest it in key.id.
	messageID := data.KeyID
	if messageID == "" {
		messageID = data.Key.ID
	}
	if messageID == "" {
		return
	}
	status, ok := mapEvolutionAckStatus(data.Status)
	if !ok {
		return
	}
	if err := h.audit.MarkOutboundStatusByExternalID(c.Context(), integrationID, messageID, status); err != nil {
		slog.WarnContext(c.Context(), "evolution status update: failed to record receipt",
			slog.String("integration_id", integrationID.String()),
			slog.String("message_id", messageID),
			slog.String("err", err.Error()))
	}
}

// mapEvolutionAckStatus translates Evolution/Baileys ack strings into the audit
// lifecycle. Baileys statuses vary across versions; we only act on the two that
// matter for a receipt (delivered/read) and ignore the rest (SERVER_ACK/PENDING
// add nothing past "sent"). ok=false means "not a status worth recording".
func mapEvolutionAckStatus(raw string) (domain.AuditStatus, bool) {
	switch strings.ToUpper(raw) {
	case "DELIVERY_ACK", "DELIVERED":
		return domain.AuditStatusDelivered, true
	case "READ", "PLAYED":
		return domain.AuditStatusRead, true
	default:
		return "", false
	}
}

func extractContent(data evolutionData) string {
	msgType := data.MessageType
	msg := data.Message

	// Fallback to nested v2 structure
	if msgType == "" && msg.MessageType != "" {
		msgType = msg.MessageType
	}
	if msg.Message != nil {
		msg = *msg.Message
	}

	switch msgType {
	case "conversation":
		return msg.Conversation
	case "extendedTextMessage":
		return msg.ExtendedTextMessage.Text
	case "listResponseMessage":
		// Customer tapped a row in a WhatsApp List Message — the row ID is
		// opaque to lango (see domain.ListRow) and flows through unchanged
		// as ButtonPayload for the consumer to interpret.
		return msg.ListResponseMessage.SingleSelectReply.SelectedRowID
	default:
		if msgType != "" {
			return fmt.Sprintf("[MEDIA:%s]", strings.ToUpper(msgType))
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
	// messages.update fields — the sent message's id and its delivery ack.
	KeyID  string `json:"keyId"`
	Status string `json:"status"`
}

type evolutionKey struct {
	RemoteJid string `json:"remoteJid"`
	FromMe    bool   `json:"fromMe"`
	ID        string `json:"id"`
	// AddressingMode is "lid" when RemoteJid is a privacy Linked ID rather than
	// a phone JID; RemoteJidAlt then carries the real "<phone>@s.whatsapp.net".
	AddressingMode string `json:"addressingMode"`
	RemoteJidAlt   string `json:"remoteJidAlt"`
}

type evolutionMessage struct {
	// Evolution API v2 nests these inside data.message for messages.upsert
	Key         evolutionKey      `json:"key"`
	MessageType string            `json:"messageType"`
	Message     *evolutionMessage `json:"message"`

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

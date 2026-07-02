package http

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // required by Twilio's signature algorithm, not used for secrecy
	"encoding/base64"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/kituomenyu/lango/internal/application"
	"github.com/kituomenyu/lango/internal/domain"
	"github.com/valyala/fasthttp"
)

// WebhookTwilioHandler handles inbound HTTP events from Twilio's WhatsApp API.
type WebhookTwilioHandler struct {
	integrations  domain.IntegrationRepository
	forward       *application.ForwardInboundUseCase
	publicBaseURL string // e.g. https://xxxx.trycloudflare.com — must match what Twilio was configured with
}

func NewWebhookTwilioHandler(integrations domain.IntegrationRepository, forward *application.ForwardInboundUseCase, publicBaseURL string) *WebhookTwilioHandler {
	return &WebhookTwilioHandler{
		integrations:  integrations,
		forward:       forward,
		publicBaseURL: strings.TrimSuffix(publicBaseURL, "/"),
	}
}

// ReceiveWebhook handles POST /webhooks/twilio/:integration_id.
// Twilio posts application/x-www-form-urlencoded, not JSON.
func (h *WebhookTwilioHandler) ReceiveWebhook(c *fiber.Ctx) error {
	integrationID, err := uuid.Parse(c.Params("integration_id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid integration_id")
	}

	params := formParams(c.Context().PostArgs())

	if h.publicBaseURL != "" {
		cfg, err := h.integrations.GetByID(c.Context(), integrationID)
		if err != nil {
			slog.WarnContext(c.Context(), "twilio webhook: integration not found",
				slog.String("integration_id", integrationID.String()))
			return fiber.NewError(fiber.StatusForbidden, "integration not configured")
		}
		url := h.publicBaseURL + "/webhooks/twilio/" + integrationID.String()
		if !verifyTwilioSignature(url, params, cfg.AccessToken, c.Get("X-Twilio-Signature")) {
			slog.WarnContext(c.Context(), "twilio webhook: signature validation failed",
				slog.String("integration_id", integrationID.String()))
			return fiber.NewError(fiber.StatusUnauthorized, "invalid webhook signature")
		}
	}

	from := strings.TrimPrefix(params["From"], "whatsapp:")
	from = strings.TrimPrefix(from, "+")

	buttonPayload := params["ButtonPayload"]
	body := params["Body"]
	if buttonPayload == "" && body == "" {
		return c.SendStatus(fiber.StatusOK)
	}

	event := domain.InboundEvent{
		From:          from,
		Content:       body,
		ButtonPayload: buttonPayload,
		ExternalID:    params["MessageSid"],
		ReceivedAt:    time.Now().UTC(),
	}
	h.forward.Execute(c.Context(), integrationID, event, correlationIDFrom(c))

	return c.SendStatus(fiber.StatusOK)
}

func formParams(args *fasthttp.Args) map[string]string {
	params := make(map[string]string, args.Len())
	args.VisitAll(func(key, value []byte) {
		params[string(key)] = string(value)
	})
	return params
}

// verifyTwilioSignature implements Twilio's request validation algorithm:
// HMAC-SHA1(AuthToken, URL + sorted(key+value for every POST param)),
// base64-encoded, compared against X-Twilio-Signature.
// https://www.twilio.com/docs/usage/webhooks/webhooks-security
func verifyTwilioSignature(fullURL string, params map[string]string, authToken, signature string) bool {
	if signature == "" || authToken == "" {
		return false
	}

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString(fullURL)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(params[k])
	}

	mac := hmac.New(sha1.New, []byte(authToken))
	mac.Write([]byte(sb.String()))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(signature))
}

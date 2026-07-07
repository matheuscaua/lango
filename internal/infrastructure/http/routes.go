package http

import (
	"github.com/gofiber/fiber/v2"
	"github.com/kituomenyu/lango/internal/domain"
	pkgmiddleware "github.com/kituomenyu/lango/pkg/middleware"
)

// RegisterRoutes wires the public webhook endpoints (called by WhatsApp
// providers, no consumer auth — identity comes from the :integration_id path
// param and each provider's own signature scheme) and the consumer-facing
// /v1 API (authenticated via X-Lango-Api-Key).
func RegisterRoutes(
	app *fiber.App,
	consumers domain.ConsumerRepository,
	webhookMeta *WebhookHandler,
	webhookEvolution *WebhookEvolutionHandler,
	webhookTwilio *WebhookTwilioHandler,
	integrations *IntegrationHandler,
	messages *MessageHandler,
	audit *AuditHandler,
) {
	// ── Public: provider webhooks ─────────────────────────────────────────────
	webhooks := app.Group("/webhooks")
	if webhookMeta != nil {
		webhooks.Get("/meta/:integration_id", webhookMeta.VerifyWebhook)
		webhooks.Post("/meta/:integration_id", webhookMeta.ReceiveWebhook)
	}
	if webhookEvolution != nil {
		webhooks.Post("/evolution/:integration_id", webhookEvolution.ReceiveWebhook)
	}
	if webhookTwilio != nil {
		webhooks.Post("/twilio/:integration_id", webhookTwilio.ReceiveWebhook)
	}

	// ── Consumer-authenticated API ────────────────────────────────────────────
	v1 := app.Group("/v1", pkgmiddleware.ConsumerAuth(consumers))

	v1.Post("/integrations", integrations.Create)
	v1.Get("/integrations", integrations.List)
	v1.Get("/integrations/:id", integrations.Get)
	v1.Get("/integrations/:id/status", integrations.Status)
	v1.Post("/integrations/:id/connect", integrations.Connect)
	v1.Post("/integrations/:id/messages", messages.Send)

	v1.Get("/audit", audit.List)
}

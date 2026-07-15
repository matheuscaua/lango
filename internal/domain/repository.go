package domain

import (
	"context"

	"github.com/google/uuid"
)

// OutboundProvider abstracts message delivery to a single WhatsApp provider
// (Meta Cloud API, Evolution API, Twilio). Implementations look up the
// Integration's credentials by IntegrationID.
type OutboundProvider interface {
	// SendMessage delivers msg and returns the provider's own message id — the
	// key a later delivery/read status webhook is matched against. Providers
	// that don't surface an id return "" (delivery receipts then simply can't be
	// correlated for that send).
	SendMessage(ctx context.Context, integrationID uuid.UUID, phone string, msg *Message) (string, error)
	SendTemplate(ctx context.Context, integrationID uuid.UUID, phone, templateName string, vars map[string]string) error
}

// MessageMetrics records outbound message telemetry per integration per
// calendar month. Implementations must be safe for concurrent use.
type MessageMetrics interface {
	IncrementSent(ctx context.Context, integrationID uuid.UUID) error
	GetMonthly(ctx context.Context, integrationID uuid.UUID, year int, month int) (int64, error)
}

// EvolutionAdmin abstracts the Evolution API's instance-management surface —
// only implemented for provider="evolution"; Meta and Twilio have their own
// manual connection flows (dashboard app review / console), no code-driven
// "connect" exists for them.
type EvolutionAdmin interface {
	CreateInstance(ctx context.Context, instanceName string) error
	SetWebhook(ctx context.Context, instanceName, webhookURL string) error
	ConnectionState(ctx context.Context, instanceName string) (string, error)
	GetQR(ctx context.Context, instanceName string) (string, error)
}

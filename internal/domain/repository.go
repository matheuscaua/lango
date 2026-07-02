package domain

import (
	"context"

	"github.com/google/uuid"
)

// OutboundProvider abstracts message delivery to a single WhatsApp provider
// (Meta Cloud API, Evolution API, Twilio). Implementations look up the
// Integration's credentials by IntegrationID.
type OutboundProvider interface {
	SendMessage(ctx context.Context, integrationID uuid.UUID, phone string, msg *Message) error
	SendTemplate(ctx context.Context, integrationID uuid.UUID, phone, templateName string, vars map[string]string) error
}

// MessageMetrics records outbound message telemetry per integration per
// calendar month. Implementations must be safe for concurrent use.
type MessageMetrics interface {
	IncrementSent(ctx context.Context, integrationID uuid.UUID) error
	GetMonthly(ctx context.Context, integrationID uuid.UUID, year int, month int) (int64, error)
}

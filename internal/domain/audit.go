package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type AuditDirection string

const (
	AuditDirectionInbound  AuditDirection = "inbound"
	AuditDirectionOutbound AuditDirection = "outbound"
)

type AuditStatus string

const (
	AuditStatusAccepted      AuditStatus = "accepted" // outbound: audit row written before calling the provider
	AuditStatusSent          AuditStatus = "sent"
	AuditStatusFailed        AuditStatus = "failed"
	AuditStatusRejected      AuditStatus = "rejected" // validation/ownership failed before dispatch
	AuditStatusReceived      AuditStatus = "received" // inbound: webhook accepted
	AuditStatusForwarded     AuditStatus = "forwarded"
	AuditStatusForwardFailed AuditStatus = "forward_failed"
)

// MessageAuditEntry is an append-only record of one send attempt or one
// inbound webhook receipt. Mirrors the pattern already used in haraka for
// order status transitions (OrderAuditEvent / OrderAuditRepository.Append) —
// never updated in place except to move Status forward through its lifecycle,
// never deleted.
type MessageAuditEntry struct {
	ID            uuid.UUID
	ConsumerID    uuid.UUID
	IntegrationID uuid.UUID
	Direction     AuditDirection
	Provider      string
	ToNumber      string // always validated E.164 before the entry is written
	FromNumber    string
	ExternalID    string
	Status        AuditStatus
	ErrorReason   string
	CorrelationID string
	CreatedAt     time.Time
}

// NewAuditEntry builds an audit entry with a fresh ID and CreatedAt.
func NewAuditEntry(consumerID, integrationID uuid.UUID, direction AuditDirection, provider, toNumber, fromNumber, externalID, correlationID string, status AuditStatus) *MessageAuditEntry {
	return &MessageAuditEntry{
		ID:            uuid.Must(uuid.NewV7()),
		ConsumerID:    consumerID,
		IntegrationID: integrationID,
		Direction:     direction,
		Provider:      provider,
		ToNumber:      toNumber,
		FromNumber:    fromNumber,
		ExternalID:    externalID,
		Status:        status,
		CorrelationID: correlationID,
		CreatedAt:     time.Now().UTC(),
	}
}

// MessageAuditRepository abstracts persistence for the audit trail.
type MessageAuditRepository interface {
	// Append writes a new audit entry (append-only).
	Append(ctx context.Context, e *MessageAuditEntry) error
	// UpdateStatus moves an existing entry's status forward (e.g. accepted -> sent/failed).
	UpdateStatus(ctx context.Context, id uuid.UUID, status AuditStatus, errorReason string) error
	// ListByConsumer returns audit entries for a consumer, optionally filtered by
	// integration and status, newest first. Empty filters are ignored.
	ListByConsumer(ctx context.Context, consumerID uuid.UUID, integrationID *uuid.UUID, status AuditStatus, limit int) ([]*MessageAuditEntry, error)
}

package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Consumer is an external service that talks to lango (e.g. "kituo-menyu-haraka").
// Every Integration belongs to exactly one Consumer — this is the boundary that
// makes cross-consumer message leakage impossible: a request authenticated as
// Consumer A can never act on an Integration owned by Consumer B.
type Consumer struct {
	ID uuid.UUID
	// Slug is a short human-readable identifier (e.g. "kituo-menyu-haraka"), unique.
	Slug string
	// APIKeyHash is the SHA-256 hash of the consumer's API key. The raw key is
	// only ever returned once, at creation time — never stored, never logged.
	APIKeyHash string
	// CallbackURL is where lango forwards inbound webhook events for this
	// consumer's integrations: POST {CallbackURL}/internal/v1/inbound.
	CallbackURL string
	// CallbackSecret authenticates lango's callback request to the consumer
	// (X-Lango-Callback-Secret header) — lets the consumer verify the inbound
	// forward genuinely came from lango.
	CallbackSecret string
	Active         bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ConsumerRepository abstracts persistence for consumers.
type ConsumerRepository interface {
	Save(ctx context.Context, c *Consumer) error
	GetByID(ctx context.Context, id uuid.UUID) (*Consumer, error)
	GetByAPIKeyHash(ctx context.Context, apiKeyHash string) (*Consumer, error)
}

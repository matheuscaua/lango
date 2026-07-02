package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Integration holds the credentials for one WhatsApp number/channel, owned by
// exactly one Consumer. This replaces haraka's tenant-scoped ChannelConfig —
// lango has no notion of "tenant"; ownership is by Consumer only.
type Integration struct {
	ID         uuid.UUID
	ConsumerID uuid.UUID
	Provider   string // "meta" | "evolution" | "twilio"
	// PhoneNumberID is provider-specific: Meta's phone_number_id, the Evolution
	// instance name, or the Twilio "From" number (E.164, no "whatsapp:" prefix).
	PhoneNumberID string
	AccessToken   string
	// VerifyToken is used by Meta's webhook challenge. Twilio repurposes this
	// field to hold the Account SID (see infrastructure/twilio).
	VerifyToken string
	Active      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// IntegrationRepository abstracts persistence for integrations.
type IntegrationRepository interface {
	Save(ctx context.Context, i *Integration) error
	GetByID(ctx context.Context, id uuid.UUID) (*Integration, error)
	ListByConsumer(ctx context.Context, consumerID uuid.UUID) ([]*Integration, error)
	Update(ctx context.Context, i *Integration) error
}

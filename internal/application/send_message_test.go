package application_test

import (
	"time"

	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kituomenyu/lango/internal/application"
	"github.com/kituomenyu/lango/internal/domain"
)

// ── Mocks ─────────────────────────────────────────────────────────────────────

type mockIntegrationRepo struct {
	integration *domain.Integration
}

func (m *mockIntegrationRepo) Save(_ context.Context, _ *domain.Integration) error { return nil }
func (m *mockIntegrationRepo) GetByID(_ context.Context, _ uuid.UUID) (*domain.Integration, error) {
	if m.integration == nil {
		return nil, domain.ErrIntegrationNotFound
	}
	return m.integration, nil
}
func (m *mockIntegrationRepo) ListByConsumer(_ context.Context, _ uuid.UUID) ([]*domain.Integration, error) {
	return nil, nil
}
func (m *mockIntegrationRepo) Update(_ context.Context, _ *domain.Integration) error { return nil }

type mockAuditRepo struct {
	entries []*domain.MessageAuditEntry
}

func (m *mockAuditRepo) Append(_ context.Context, e *domain.MessageAuditEntry) error {
	m.entries = append(m.entries, e)
	return nil
}
func (m *mockAuditRepo) UpdateStatus(_ context.Context, id uuid.UUID, status domain.AuditStatus, reason string) error {
	for _, e := range m.entries {
		if e.ID == id {
			e.Status = status
			e.ErrorReason = reason
		}
	}
	return nil
}
func (m *mockAuditRepo) MarkSent(_ context.Context, id uuid.UUID, externalID string) error {
	for _, e := range m.entries {
		if e.ID == id {
			e.Status = domain.AuditStatusSent
			e.ExternalID = externalID
		}
	}
	return nil
}
func (m *mockAuditRepo) MarkOutboundStatusByExternalID(_ context.Context, _ uuid.UUID, externalID string, status domain.AuditStatus) error {
	for _, e := range m.entries {
		if e.ExternalID == externalID && e.Direction == domain.AuditDirectionOutbound {
			e.Status = status
		}
	}
	return nil
}
func (m *mockAuditRepo) ListByConsumer(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _ domain.AuditStatus, _ int) ([]*domain.MessageAuditEntry, error) {
	return m.entries, nil
}

func (m *mockAuditRepo) SummarizeIntegration(_ context.Context, _ uuid.UUID, _ time.Time) (*domain.IntegrationActivitySummary, error) {
	return &domain.IntegrationActivitySummary{}, nil
}

type mockProvider struct {
	sendErr   error
	sent      bool
	messageID string
}

func (m *mockProvider) SendMessage(_ context.Context, _ uuid.UUID, _ string, _ *domain.Message) (string, error) {
	m.sent = true
	return m.messageID, m.sendErr
}
func (m *mockProvider) SendTemplate(_ context.Context, _ uuid.UUID, _, _ string, _ map[string]string) error {
	return nil
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestSendMessage_RejectsCrossConsumerAccess(t *testing.T) {
	ownerConsumerID := uuid.New()
	attackerConsumerID := uuid.New()
	integrationID := uuid.New()

	integrations := &mockIntegrationRepo{integration: &domain.Integration{
		ID: integrationID, ConsumerID: ownerConsumerID, Provider: "twilio", Active: true,
	}}
	audit := &mockAuditRepo{}
	provider := &mockProvider{}

	uc := application.NewSendMessageUseCase(integrations, audit, map[string]domain.OutboundProvider{"twilio": provider})

	err := uc.Execute(context.Background(), application.SendMessageInput{
		ConsumerID:    attackerConsumerID,
		IntegrationID: integrationID,
		Phone:         "+5511999990000",
		Message:       &domain.Message{Type: domain.TypeText, Content: "oi"},
	})

	if !errors.Is(err, domain.ErrIntegrationNotOwned) {
		t.Fatalf("expected ErrIntegrationNotOwned, got %v", err)
	}
	if provider.sent {
		t.Error("provider.SendMessage must never be called for a cross-consumer request")
	}
	if len(audit.entries) != 1 || audit.entries[0].Status != domain.AuditStatusRejected {
		t.Fatalf("expected exactly 1 rejected audit entry, got %+v", audit.entries)
	}
}

func TestSendMessage_RejectsInvalidPhoneFormat(t *testing.T) {
	consumerID := uuid.New()
	integrationID := uuid.New()

	integrations := &mockIntegrationRepo{integration: &domain.Integration{
		ID: integrationID, ConsumerID: consumerID, Provider: "twilio", Active: true,
	}}
	audit := &mockAuditRepo{}
	provider := &mockProvider{}

	uc := application.NewSendMessageUseCase(integrations, audit, map[string]domain.OutboundProvider{"twilio": provider})

	err := uc.Execute(context.Background(), application.SendMessageInput{
		ConsumerID:    consumerID,
		IntegrationID: integrationID,
		Phone:         "5511999990000", // missing "+" — exactly the bug found in the haraka<->kituo-menyu integration
		Message:       &domain.Message{Type: domain.TypeText, Content: "oi"},
	})

	if !errors.Is(err, domain.ErrInvalidPhoneNumber) {
		t.Fatalf("expected ErrInvalidPhoneNumber, got %v", err)
	}
	if provider.sent {
		t.Error("provider.SendMessage must never be called for a malformed number")
	}
}

func TestSendMessage_Success_RecordsAcceptedThenSent(t *testing.T) {
	consumerID := uuid.New()
	integrationID := uuid.New()

	integrations := &mockIntegrationRepo{integration: &domain.Integration{
		ID: integrationID, ConsumerID: consumerID, Provider: "twilio", Active: true,
	}}
	audit := &mockAuditRepo{}
	provider := &mockProvider{messageID: "SM123abc"}

	uc := application.NewSendMessageUseCase(integrations, audit, map[string]domain.OutboundProvider{"twilio": provider})

	err := uc.Execute(context.Background(), application.SendMessageInput{
		ConsumerID:    consumerID,
		IntegrationID: integrationID,
		Phone:         "+5511999990000",
		Message:       &domain.Message{Type: domain.TypeText, Content: "oi"},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !provider.sent {
		t.Error("expected provider.SendMessage to be called")
	}
	if len(audit.entries) != 1 || audit.entries[0].Status != domain.AuditStatusSent {
		t.Fatalf("expected exactly 1 sent audit entry, got %+v", audit.entries)
	}
	// The provider's message id must be persisted so a later delivery/read
	// status webhook can correlate back to this send.
	if audit.entries[0].ExternalID != "SM123abc" {
		t.Fatalf("expected provider message id stored as external_id, got %q", audit.entries[0].ExternalID)
	}
}

func TestSendMessage_ProviderFailure_RecordsFailed(t *testing.T) {
	consumerID := uuid.New()
	integrationID := uuid.New()

	integrations := &mockIntegrationRepo{integration: &domain.Integration{
		ID: integrationID, ConsumerID: consumerID, Provider: "twilio", Active: true,
	}}
	audit := &mockAuditRepo{}
	provider := &mockProvider{sendErr: errors.New("provider unreachable")}

	uc := application.NewSendMessageUseCase(integrations, audit, map[string]domain.OutboundProvider{"twilio": provider})

	err := uc.Execute(context.Background(), application.SendMessageInput{
		ConsumerID:    consumerID,
		IntegrationID: integrationID,
		Phone:         "+5511999990000",
		Message:       &domain.Message{Type: domain.TypeText, Content: "oi"},
	})

	if err == nil {
		t.Fatal("expected error to propagate from provider")
	}
	if len(audit.entries) != 1 || audit.entries[0].Status != domain.AuditStatusFailed {
		t.Fatalf("expected exactly 1 failed audit entry, got %+v", audit.entries)
	}
}

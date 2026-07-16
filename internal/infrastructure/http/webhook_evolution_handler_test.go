package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/kituomenyu/lango/internal/application"
	"github.com/kituomenyu/lango/internal/domain"
)

type mockIntegrationRepo struct {
	domain.IntegrationRepository
	integration *domain.Integration
}

func (m *mockIntegrationRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Integration, error) {
	if m.integration != nil {
		return m.integration, nil
	}
	return &domain.Integration{
		ID:           id,
		ConsumerID:   uuid.New(),
		Provider:     "evolution",
		PhoneNumberID: "test-instance",
	}, nil
}

type mockConsumerRepo struct {
	domain.ConsumerRepository
	url string
}

func (m *mockConsumerRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Consumer, error) {
	return &domain.Consumer{
		ID:             id,
		CallbackURL:    m.url,
		CallbackSecret: "secret",
	}, nil
}

type mockAuditRepo struct {
	domain.MessageAuditRepository
	appended []*domain.MessageAuditEntry
	statuses map[uuid.UUID]domain.AuditStatus
}

func (m *mockAuditRepo) Append(ctx context.Context, entry *domain.MessageAuditEntry) error {
	m.appended = append(m.appended, entry)
	if m.statuses == nil {
		m.statuses = make(map[uuid.UUID]domain.AuditStatus)
	}
	m.statuses[entry.ID] = entry.Status
	return nil
}

func (m *mockAuditRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.AuditStatus, errStr string) error {
	if m.statuses != nil {
		m.statuses[id] = status
	}
	return nil
}

func TestWebhookEvolutionHandler_Filters(t *testing.T) {
	integrationID := uuid.New()

	tests := []struct {
		name             string
		maxAgeSecs       int
		allowedPhones    []string
		payloadTimestamp int64
		payloadPhone     string
		wantForwarded    bool
	}{
		{
			name:             "no filters -> passes",
			maxAgeSecs:       0,
			allowedPhones:    nil,
			payloadTimestamp: time.Now().Unix() - 500, // old
			payloadPhone:     "5511999999999",
			wantForwarded:    true,
		},
		{
			name:             "fresh message within max age -> passes",
			maxAgeSecs:       120,
			allowedPhones:    nil,
			payloadTimestamp: time.Now().Unix() - 10, // 10s old
			payloadPhone:     "5511999999999",
			wantForwarded:    true,
		},
		{
			name:             "stale message older than max age -> filtered",
			maxAgeSecs:       120,
			allowedPhones:    nil,
			payloadTimestamp: time.Now().Unix() - 300, // 5 min old
			payloadPhone:     "5511999999999",
			wantForwarded:    false,
		},
		{
			name:             "phone in allowlist -> passes",
			maxAgeSecs:       0,
			allowedPhones:    []string{"5511999999999"},
			payloadTimestamp: 0,
			payloadPhone:     "5511999999999",
			wantForwarded:    true,
		},
		{
			name:             "phone not in allowlist -> filtered",
			maxAgeSecs:       0,
			allowedPhones:    []string{"5511999999999"},
			payloadTimestamp: 0,
			payloadPhone:     "5511888888888",
			wantForwarded:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer ts.Close()

			integrationRepo := &mockIntegrationRepo{}
			consumerRepo := &mockConsumerRepo{url: ts.URL}
			auditRepo := &mockAuditRepo{}

			forwardUC := application.NewForwardInboundUseCase(integrationRepo, consumerRepo, auditRepo)

			// We need a fiber app to route the request
			app := fiber.New()
			handler := NewWebhookEvolutionHandler(forwardUC, auditRepo, tt.maxAgeSecs, tt.allowedPhones, false)
			app.Post("/webhooks/evolution/:integration_id", handler.ReceiveWebhook)

			// Construct payload
			dataPayload := evolutionWebhookPayload{
				Event:    "messages.upsert",
				Instance: "test-instance",
				Data: evolutionData{
					Key: evolutionKey{
						RemoteJid: tt.payloadPhone + "@s.whatsapp.net",
						FromMe:    false,
						ID:        "msg-123",
					},
					MessageType:      "conversation",
					MessageTimestamp:  tt.payloadTimestamp,
					Message: evolutionMessage{
						Conversation: "Hello",
					},
				},
			}

			body, _ := json.Marshal(dataPayload)
			req := httptest.NewRequest(http.MethodPost, "/webhooks/evolution/"+integrationID.String(), bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}

			if resp.StatusCode != http.StatusOK {
				t.Errorf("expected 200 OK, got %d", resp.StatusCode)
			}

			// If it was forwarded, auditRepo will have an appended entry
			hasAuditEntry := len(auditRepo.appended) > 0
			if hasAuditEntry != tt.wantForwarded {
				t.Errorf("got forwarded = %v, want %v", hasAuditEntry, tt.wantForwarded)
			}
		})
	}
}

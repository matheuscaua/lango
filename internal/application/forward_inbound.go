package application

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/kituomenyu/lango/internal/domain"
)

const forwardTimeout = 5 * time.Second

// ForwardInboundUseCase normalizes a webhook message into domain.InboundEvent
// and forwards it via HTTP callback to the owning consumer — the "lango is a
// dumb pipe" half of ADR 008: it never interprets Content or ButtonPayload,
// just routes them to whichever consumer owns the integration, and audits
// both the receipt and the forward outcome.
type ForwardInboundUseCase struct {
	integrations domain.IntegrationRepository
	consumers    domain.ConsumerRepository
	audit        domain.MessageAuditRepository
	httpClient   *http.Client
}

func NewForwardInboundUseCase(integrations domain.IntegrationRepository, consumers domain.ConsumerRepository, audit domain.MessageAuditRepository) *ForwardInboundUseCase {
	return &ForwardInboundUseCase{
		integrations: integrations,
		consumers:    consumers,
		audit:        audit,
		httpClient:   &http.Client{Timeout: forwardTimeout},
	}
}

// Execute looks up the integration's owning consumer and POSTs the event to
// its callback URL. Errors are logged via the audit trail, never returned to
// the provider's webhook caller — providers retry on non-2xx, and a forward
// failure here is a lango↔consumer problem, not a "the webhook was invalid"
// problem.
func (uc *ForwardInboundUseCase) Execute(ctx context.Context, integrationID uuid.UUID, event domain.InboundEvent, correlationID string) {
	integration, err := uc.integrations.GetByID(ctx, integrationID)
	if err != nil {
		return
	}

	receivedEntry := domain.NewAuditEntry(integration.ConsumerID, integrationID, domain.AuditDirectionInbound,
		integration.Provider, integration.PhoneNumberID, event.From, event.ExternalID, correlationID, domain.AuditStatusReceived)
	_ = uc.audit.Append(ctx, receivedEntry)

	consumer, err := uc.consumers.GetByID(ctx, integration.ConsumerID)
	if err != nil || consumer.CallbackURL == "" {
		_ = uc.audit.UpdateStatus(ctx, receivedEntry.ID, domain.AuditStatusForwardFailed, "consumer has no callback_url configured")
		return
	}

	event.IntegrationID = integrationID.String()
	body, err := json.Marshal(event)
	if err != nil {
		_ = uc.audit.UpdateStatus(ctx, receivedEntry.ID, domain.AuditStatusForwardFailed, fmt.Sprintf("marshal event: %v", err))
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, consumer.CallbackURL+"/internal/v1/inbound", bytes.NewReader(body))
	if err != nil {
		_ = uc.audit.UpdateStatus(ctx, receivedEntry.ID, domain.AuditStatusForwardFailed, fmt.Sprintf("build request: %v", err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Lango-Callback-Secret", consumer.CallbackSecret)
	req.Header.Set("X-Correlation-Id", correlationID)

	resp, err := uc.httpClient.Do(req)
	if err != nil {
		_ = uc.audit.UpdateStatus(ctx, receivedEntry.ID, domain.AuditStatusForwardFailed, err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		_ = uc.audit.UpdateStatus(ctx, receivedEntry.ID, domain.AuditStatusForwardFailed, fmt.Sprintf("consumer callback returned HTTP %d", resp.StatusCode))
		return
	}

	_ = uc.audit.UpdateStatus(ctx, receivedEntry.ID, domain.AuditStatusForwarded, "")
}

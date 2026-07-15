package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kituomenyu/lango/internal/domain"
)

// SendMessageInput carries a send request already authenticated as ConsumerID.
type SendMessageInput struct {
	ConsumerID    uuid.UUID
	IntegrationID uuid.UUID
	Phone         string
	Message       *domain.Message
	CorrelationID string
}

// SendMessageUseCase validates ownership and the destination number, dispatches
// through the integration's provider, and records every attempt in the audit
// trail — before dispatch (accepted/rejected) and after (sent/failed). This is
// the enforcement point for ADR 008's "assertividade": a consumer can never
// send through an integration it doesn't own, and a malformed number is
// rejected here rather than trusted through to the provider.
type SendMessageUseCase struct {
	integrations domain.IntegrationRepository
	audit        domain.MessageAuditRepository
	providers    map[string]domain.OutboundProvider // keyed by Integration.Provider
}

func NewSendMessageUseCase(integrations domain.IntegrationRepository, audit domain.MessageAuditRepository, providers map[string]domain.OutboundProvider) *SendMessageUseCase {
	return &SendMessageUseCase{integrations: integrations, audit: audit, providers: providers}
}

func (uc *SendMessageUseCase) Execute(ctx context.Context, in SendMessageInput) error {
	integration, err := uc.integrations.GetByID(ctx, in.IntegrationID)
	if err != nil {
		return fmt.Errorf("send message: get integration: %w", err)
	}

	if integration.ConsumerID != in.ConsumerID {
		uc.rejectAndAudit(ctx, in, integration.Provider, domain.ErrIntegrationNotOwned)
		return domain.ErrIntegrationNotOwned
	}

	if !domain.IsE164(in.Phone) {
		uc.rejectAndAudit(ctx, in, integration.Provider, domain.ErrInvalidPhoneNumber)
		return domain.ErrInvalidPhoneNumber
	}

	provider, ok := uc.providers[integration.Provider]
	if !ok {
		uc.rejectAndAudit(ctx, in, integration.Provider, domain.ErrUnsupportedProvider)
		return domain.ErrUnsupportedProvider
	}

	entry := domain.NewAuditEntry(in.ConsumerID, in.IntegrationID, domain.AuditDirectionOutbound,
		integration.Provider, in.Phone, integration.PhoneNumberID, "", in.CorrelationID, domain.AuditStatusAccepted)
	if err := uc.audit.Append(ctx, entry); err != nil {
		return fmt.Errorf("send message: append audit: %w", err)
	}

	providerMessageID, sendErr := provider.SendMessage(ctx, in.IntegrationID, in.Phone, in.Message)
	if sendErr != nil {
		_ = uc.audit.UpdateStatus(ctx, entry.ID, domain.AuditStatusFailed, sendErr.Error())
		return fmt.Errorf("send message: %w", sendErr)
	}

	// Store the provider's message id alongside status=sent so a later
	// delivery/read status webhook can find this entry (MarkOutboundStatusByExternalID).
	_ = uc.audit.MarkSent(ctx, entry.ID, providerMessageID)
	return nil
}

func (uc *SendMessageUseCase) rejectAndAudit(ctx context.Context, in SendMessageInput, provider string, reason error) {
	entry := domain.NewAuditEntry(in.ConsumerID, in.IntegrationID, domain.AuditDirectionOutbound,
		provider, in.Phone, "", "", in.CorrelationID, domain.AuditStatusRejected)
	entry.ErrorReason = errStr(reason)
	_ = uc.audit.Append(ctx, entry)
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, domain.ErrIntegrationNotOwned) {
		return "integration does not belong to authenticated consumer"
	}
	return err.Error()
}

package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kituomenyu/lango/internal/domain"
)

// ConnectIntegrationInput identifies the integration to connect (or refresh
// the QR code for), already authenticated as ConsumerID.
type ConnectIntegrationInput struct {
	ConsumerID    uuid.UUID
	IntegrationID uuid.UUID
}

// ConnectState is the state machine the frontend polls against.
type ConnectState string

const (
	// ConnectStateConnecting means the instance exists and a QR is available
	// to scan — callers should keep polling (QR expires every ~20-30s, so a
	// fresh one accompanies every "connecting" response).
	ConnectStateConnecting ConnectState = "connecting"
	// ConnectStateConnected means WhatsApp is linked — no more polling needed.
	ConnectStateConnected ConnectState = "connected"
)

// ConnectIntegrationResult is what the HTTP layer serializes back to the caller.
type ConnectIntegrationResult struct {
	State    ConnectState
	QRBase64 string // empty when State == connected
}

// ConnectIntegrationUseCase drives the Evolution API "link this WhatsApp
// number" flow: create the instance (idempotent), point its webhook back at
// this lango instance, and report whether it's already connected or return a
// fresh QR code to scan. Callers (haraka today) poll this repeatedly — every
// call refreshes the QR since Baileys QR codes are short-lived.
type ConnectIntegrationUseCase struct {
	integrations domain.IntegrationRepository
	evolution    domain.EvolutionAdmin
	webhookURL   func(integrationID uuid.UUID) string
}

func NewConnectIntegrationUseCase(
	integrations domain.IntegrationRepository,
	evolution domain.EvolutionAdmin,
	webhookURL func(integrationID uuid.UUID) string,
) *ConnectIntegrationUseCase {
	return &ConnectIntegrationUseCase{integrations: integrations, evolution: evolution, webhookURL: webhookURL}
}

// ErrConnectUnsupportedProvider means Execute was called for an integration
// whose provider isn't Evolution — Meta/Twilio have no code-driven connect step.
var ErrConnectUnsupportedProvider = errors.New("connect flow is only supported for provider=evolution")

func (uc *ConnectIntegrationUseCase) Execute(ctx context.Context, in ConnectIntegrationInput) (*ConnectIntegrationResult, error) {
	integration, err := uc.integrations.GetByID(ctx, in.IntegrationID)
	if err != nil {
		return nil, fmt.Errorf("connect integration: get integration: %w", err)
	}
	if integration.ConsumerID != in.ConsumerID {
		return nil, domain.ErrIntegrationNotOwned
	}
	if integration.Provider != "evolution" {
		return nil, ErrConnectUnsupportedProvider
	}

	// instanceName == integration ID: stable, unique, and lango-native (no
	// dependency on any concept from the consumer's own domain, e.g. tenant).
	instanceName := integration.PhoneNumberID

	if err := uc.evolution.CreateInstance(ctx, instanceName); err != nil {
		return nil, fmt.Errorf("connect integration: create instance: %w", err)
	}
	if err := uc.evolution.SetWebhook(ctx, instanceName, uc.webhookURL(integration.ID)); err != nil {
		return nil, fmt.Errorf("connect integration: set webhook: %w", err)
	}

	state, err := uc.evolution.ConnectionState(ctx, instanceName)
	if err != nil {
		return nil, fmt.Errorf("connect integration: connection state: %w", err)
	}

	if state == "open" {
		if !integration.Active {
			integration.Active = true
			if updErr := uc.integrations.Update(ctx, integration); updErr != nil {
				return nil, fmt.Errorf("connect integration: activate: %w", updErr)
			}
		}
		return &ConnectIntegrationResult{State: ConnectStateConnected}, nil
	}

	qr, err := uc.evolution.GetQR(ctx, instanceName)
	if err != nil {
		if errors.Is(err, domain.ErrConnectQRNotAvailable) {
			// Instance transitioned between our state check and QR fetch —
			// harmless race, ask the caller to poll again shortly.
			return &ConnectIntegrationResult{State: ConnectStateConnecting}, nil
		}
		return nil, fmt.Errorf("connect integration: get QR: %w", err)
	}

	return &ConnectIntegrationResult{State: ConnectStateConnecting, QRBase64: qr}, nil
}

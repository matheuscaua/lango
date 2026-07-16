package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kituomenyu/lango/internal/application"
	"github.com/kituomenyu/lango/internal/domain"
)

type mockEvolutionAdmin struct {
	state         string
	stateErr      error
	qr            string
	qrErr         error
	createCalled  bool
	webhookURL    string
	logoutCalled  bool
	deleteCalled  bool
}

func (m *mockEvolutionAdmin) CreateInstance(_ context.Context, _ string) error {
	m.createCalled = true
	return nil
}
func (m *mockEvolutionAdmin) SetWebhook(_ context.Context, _ string, webhookURL string) error {
	m.webhookURL = webhookURL
	return nil
}
func (m *mockEvolutionAdmin) ConnectionState(_ context.Context, _ string) (string, error) {
	return m.state, m.stateErr
}
func (m *mockEvolutionAdmin) GetQR(_ context.Context, _ string) (string, error) {
	return m.qr, m.qrErr
}
func (m *mockEvolutionAdmin) LogoutInstance(_ context.Context, _ string) error {
	m.logoutCalled = true
	return nil
}
func (m *mockEvolutionAdmin) DeleteInstance(_ context.Context, _ string) error {
	m.deleteCalled = true
	return nil
}

func newConnectUC(integrations *mockIntegrationRepo, admin *mockEvolutionAdmin) *application.ConnectIntegrationUseCase {
	return application.NewConnectIntegrationUseCase(integrations, admin, func(id uuid.UUID) string {
		return "http://host.docker.internal:3100/webhooks/evolution/" + id.String()
	})
}

func TestConnectIntegration_RejectsCrossConsumerAccess(t *testing.T) {
	ownerID := uuid.New()
	attackerID := uuid.New()
	integrationID := uuid.New()

	integrations := &mockIntegrationRepo{integration: &domain.Integration{
		ID: integrationID, ConsumerID: ownerID, Provider: "evolution", PhoneNumberID: integrationID.String(),
	}}
	admin := &mockEvolutionAdmin{state: "close"}
	uc := newConnectUC(integrations, admin)

	_, err := uc.Execute(context.Background(), application.ConnectIntegrationInput{
		ConsumerID: attackerID, IntegrationID: integrationID,
	})

	if !errors.Is(err, domain.ErrIntegrationNotOwned) {
		t.Fatalf("expected ErrIntegrationNotOwned, got %v", err)
	}
	if admin.createCalled {
		t.Error("must never touch Evolution for a cross-consumer request")
	}
}

func TestConnectIntegration_RejectsNonEvolutionProvider(t *testing.T) {
	consumerID := uuid.New()
	integrationID := uuid.New()

	integrations := &mockIntegrationRepo{integration: &domain.Integration{
		ID: integrationID, ConsumerID: consumerID, Provider: "twilio",
	}}
	admin := &mockEvolutionAdmin{}
	uc := newConnectUC(integrations, admin)

	_, err := uc.Execute(context.Background(), application.ConnectIntegrationInput{
		ConsumerID: consumerID, IntegrationID: integrationID,
	})

	if !errors.Is(err, application.ErrConnectUnsupportedProvider) {
		t.Fatalf("expected ErrConnectUnsupportedProvider, got %v", err)
	}
}

func TestConnectIntegration_NotYetConnected_ReturnsQR(t *testing.T) {
	consumerID := uuid.New()
	integrationID := uuid.New()

	integrations := &mockIntegrationRepo{integration: &domain.Integration{
		ID: integrationID, ConsumerID: consumerID, Provider: "evolution",
		PhoneNumberID: integrationID.String(), Active: false,
	}}
	admin := &mockEvolutionAdmin{state: "connecting", qr: "base64pngdata"}
	uc := newConnectUC(integrations, admin)

	result, err := uc.Execute(context.Background(), application.ConnectIntegrationInput{
		ConsumerID: consumerID, IntegrationID: integrationID,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State != application.ConnectStateConnecting {
		t.Errorf("expected connecting state, got %s", result.State)
	}
	if result.QRBase64 != "base64pngdata" {
		t.Errorf("expected QR to be returned, got %q", result.QRBase64)
	}
	if !admin.createCalled {
		t.Error("expected CreateInstance to be called")
	}
	wantWebhook := "http://host.docker.internal:3100/webhooks/evolution/" + integrationID.String()
	if admin.webhookURL != wantWebhook {
		t.Errorf("expected webhook %q, got %q", wantWebhook, admin.webhookURL)
	}
}

func TestConnectIntegration_AlreadyOpen_ActivatesAndReturnsConnected(t *testing.T) {
	consumerID := uuid.New()
	integrationID := uuid.New()

	integrations := &mockIntegrationRepo{integration: &domain.Integration{
		ID: integrationID, ConsumerID: consumerID, Provider: "evolution",
		PhoneNumberID: integrationID.String(), Active: false,
	}}
	admin := &mockEvolutionAdmin{state: "open"}
	uc := newConnectUC(integrations, admin)

	result, err := uc.Execute(context.Background(), application.ConnectIntegrationInput{
		ConsumerID: consumerID, IntegrationID: integrationID,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State != application.ConnectStateConnected {
		t.Errorf("expected connected state, got %s", result.State)
	}
	if result.QRBase64 != "" {
		t.Errorf("expected no QR once connected, got %q", result.QRBase64)
	}
	if !integrations.integration.Active {
		t.Error("expected integration to be marked Active after connecting")
	}
}

package application

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/kituomenyu/lango/internal/domain"
)

// DisconnectIntegrationInput identifies the integration to disconnect, already
// authenticated as ConsumerID.
type DisconnectIntegrationInput struct {
	ConsumerID    uuid.UUID
	IntegrationID uuid.UUID
}

// DisconnectIntegrationUseCase tears down a WhatsApp integration: for Evolution
// it ends the linked WhatsApp session and removes the instance (so an "old"
// number is actually disconnected, not just forgotten), then deletes the
// integration row. The Evolution teardown is best-effort — a gateway that's
// already down must not block removing the integration, otherwise a broken
// instance could never be cleaned up.
type DisconnectIntegrationUseCase struct {
	integrations domain.IntegrationRepository
	evolution    domain.EvolutionAdmin
}

func NewDisconnectIntegrationUseCase(
	integrations domain.IntegrationRepository,
	evolution domain.EvolutionAdmin,
) *DisconnectIntegrationUseCase {
	return &DisconnectIntegrationUseCase{integrations: integrations, evolution: evolution}
}

func (uc *DisconnectIntegrationUseCase) Execute(ctx context.Context, in DisconnectIntegrationInput) error {
	integration, err := uc.integrations.GetByID(ctx, in.IntegrationID)
	if err != nil {
		return fmt.Errorf("disconnect integration: get integration: %w", err)
	}
	if integration.ConsumerID != in.ConsumerID {
		return domain.ErrIntegrationNotOwned
	}

	// Evolution: unpair the phone and drop the instance. Best-effort — log and
	// continue so the integration row is still removed even if the gateway is
	// unreachable or the instance is already gone.
	if integration.Provider == "evolution" {
		instanceName := integration.PhoneNumberID // instanceName == integration ID (see ConnectIntegrationUseCase)
		if err := uc.evolution.LogoutInstance(ctx, instanceName); err != nil {
			slog.WarnContext(ctx, "disconnect: evolution logout failed (continuing)",
				slog.String("integration_id", in.IntegrationID.String()), slog.String("err", err.Error()))
		}
		if err := uc.evolution.DeleteInstance(ctx, instanceName); err != nil {
			slog.WarnContext(ctx, "disconnect: evolution delete instance failed (continuing)",
				slog.String("integration_id", in.IntegrationID.String()), slog.String("err", err.Error()))
		}
	}

	if err := uc.integrations.Delete(ctx, in.IntegrationID); err != nil {
		return fmt.Errorf("disconnect integration: delete: %w", err)
	}
	return nil
}

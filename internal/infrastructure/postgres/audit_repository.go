package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kituomenyu/lango/internal/domain"
)

// AuditRepository implements domain.MessageAuditRepository using PostgreSQL.
// Append-only by convention: no method deletes or overwrites a row wholesale,
// only UpdateStatus advances `status`.
type AuditRepository struct {
	db *pgxpool.Pool
}

func NewAuditRepository(db *pgxpool.Pool) *AuditRepository {
	return &AuditRepository{db: db}
}

func (r *AuditRepository) Append(ctx context.Context, e *domain.MessageAuditEntry) error {
	const q = `
		INSERT INTO message_audit_entries
			(id, consumer_id, integration_id, direction, provider, to_number, from_number,
			 external_id, status, error_reason, correlation_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`
	_, err := r.db.Exec(ctx, q,
		e.ID, e.ConsumerID, e.IntegrationID, string(e.Direction), e.Provider,
		e.ToNumber, e.FromNumber, e.ExternalID, string(e.Status), e.ErrorReason,
		e.CorrelationID, e.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("append audit entry: %w", err)
	}
	return nil
}

func (r *AuditRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.AuditStatus, errorReason string) error {
	const q = `UPDATE message_audit_entries SET status = $2, error_reason = $3 WHERE id = $1`
	_, err := r.db.Exec(ctx, q, id, string(status), errorReason)
	if err != nil {
		return fmt.Errorf("update audit entry status: %w", err)
	}
	return nil
}

func (r *AuditRepository) ListByConsumer(ctx context.Context, consumerID uuid.UUID, integrationID *uuid.UUID, status domain.AuditStatus, limit int) ([]*domain.MessageAuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `
		SELECT id, consumer_id, integration_id, direction, provider, to_number, from_number,
		       external_id, status, error_reason, correlation_id, created_at
		FROM message_audit_entries
		WHERE consumer_id = $1
		  AND ($2::uuid IS NULL OR integration_id = $2)
		  AND ($3::text = '' OR status = $3)
		ORDER BY created_at DESC
		LIMIT $4`

	var integrationFilter *uuid.UUID
	if integrationID != nil {
		integrationFilter = integrationID
	}

	rows, err := r.db.Query(ctx, q, consumerID, integrationFilter, string(status), limit)
	if err != nil {
		return nil, fmt.Errorf("list audit entries: %w", err)
	}
	defer rows.Close()

	var out []*domain.MessageAuditEntry
	for rows.Next() {
		var e domain.MessageAuditEntry
		var direction, st string
		if err := rows.Scan(&e.ID, &e.ConsumerID, &e.IntegrationID, &direction, &e.Provider,
			&e.ToNumber, &e.FromNumber, &e.ExternalID, &st, &e.ErrorReason,
			&e.CorrelationID, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit entry: %w", err)
		}
		e.Direction = domain.AuditDirection(direction)
		e.Status = domain.AuditStatus(st)
		out = append(out, &e)
	}
	return out, rows.Err()
}

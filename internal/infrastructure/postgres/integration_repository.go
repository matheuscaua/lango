package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kituomenyu/lango/internal/domain"
)

// IntegrationRepository implements domain.IntegrationRepository using PostgreSQL.
type IntegrationRepository struct {
	db *pgxpool.Pool
}

func NewIntegrationRepository(db *pgxpool.Pool) *IntegrationRepository {
	return &IntegrationRepository{db: db}
}

func (r *IntegrationRepository) Save(ctx context.Context, i *domain.Integration) error {
	if i.ID == uuid.Nil {
		i.ID = uuid.New()
	}
	now := time.Now().UTC()
	i.CreatedAt, i.UpdatedAt = now, now
	const q = `
		INSERT INTO integrations (id, consumer_id, provider, phone_number_id, access_token, verify_token, active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err := r.db.Exec(ctx, q, i.ID, i.ConsumerID, i.Provider, i.PhoneNumberID, i.AccessToken, i.VerifyToken, i.Active, i.CreatedAt, i.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save integration: %w", err)
	}
	return nil
}

func (r *IntegrationRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Integration, error) {
	const q = `
		SELECT id, consumer_id, provider, phone_number_id, access_token, verify_token, active, created_at, updated_at
		FROM integrations WHERE id = $1`
	return scanIntegration(r.db.QueryRow(ctx, q, id))
}

func (r *IntegrationRepository) ListByConsumer(ctx context.Context, consumerID uuid.UUID) ([]*domain.Integration, error) {
	const q = `
		SELECT id, consumer_id, provider, phone_number_id, access_token, verify_token, active, created_at, updated_at
		FROM integrations WHERE consumer_id = $1 ORDER BY created_at ASC`
	rows, err := r.db.Query(ctx, q, consumerID)
	if err != nil {
		return nil, fmt.Errorf("list integrations: %w", err)
	}
	defer rows.Close()

	var out []*domain.Integration
	for rows.Next() {
		i, err := scanIntegrationRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (r *IntegrationRepository) Update(ctx context.Context, i *domain.Integration) error {
	const q = `
		UPDATE integrations
		SET provider = $2, phone_number_id = $3, access_token = $4, verify_token = $5, active = $6, updated_at = NOW()
		WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, i.ID, i.Provider, i.PhoneNumberID, i.AccessToken, i.VerifyToken, i.Active)
	if err != nil {
		return fmt.Errorf("update integration: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrIntegrationNotFound
	}
	return nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanIntegration(row pgx.Row) (*domain.Integration, error) {
	i, err := scanIntegrationRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrIntegrationNotFound
		}
		return nil, err
	}
	return i, nil
}

func scanIntegrationRow(row scannable) (*domain.Integration, error) {
	var i domain.Integration
	err := row.Scan(&i.ID, &i.ConsumerID, &i.Provider, &i.PhoneNumberID, &i.AccessToken, &i.VerifyToken,
		&i.Active, &i.CreatedAt, &i.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("integration scan: %w", err)
	}
	return &i, nil
}

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

// ConsumerRepository implements domain.ConsumerRepository using PostgreSQL.
type ConsumerRepository struct {
	db *pgxpool.Pool
}

func NewConsumerRepository(db *pgxpool.Pool) *ConsumerRepository {
	return &ConsumerRepository{db: db}
}

func (r *ConsumerRepository) Save(ctx context.Context, c *domain.Consumer) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	now := time.Now().UTC()
	c.CreatedAt, c.UpdatedAt = now, now
	const q = `
		INSERT INTO consumers (id, slug, api_key_hash, callback_url, callback_secret, active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err := r.db.Exec(ctx, q, c.ID, c.Slug, c.APIKeyHash, c.CallbackURL, c.CallbackSecret, c.Active, c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save consumer: %w", err)
	}
	return nil
}

func (r *ConsumerRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Consumer, error) {
	const q = `
		SELECT id, slug, api_key_hash, callback_url, callback_secret, active, created_at, updated_at
		FROM consumers WHERE id = $1`
	return scanConsumer(r.db.QueryRow(ctx, q, id))
}

func (r *ConsumerRepository) GetByAPIKeyHash(ctx context.Context, apiKeyHash string) (*domain.Consumer, error) {
	const q = `
		SELECT id, slug, api_key_hash, callback_url, callback_secret, active, created_at, updated_at
		FROM consumers WHERE api_key_hash = $1`
	return scanConsumer(r.db.QueryRow(ctx, q, apiKeyHash))
}

func scanConsumer(row pgx.Row) (*domain.Consumer, error) {
	var c domain.Consumer
	err := row.Scan(&c.ID, &c.Slug, &c.APIKeyHash, &c.CallbackURL, &c.CallbackSecret,
		&c.Active, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrConsumerNotFound
		}
		return nil, fmt.Errorf("consumer scan: %w", err)
	}
	return &c, nil
}

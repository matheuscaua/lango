// Package redis provides Redis-backed infrastructure implementations.
package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

const metricsTTL = 13 * 30 * 24 * time.Hour // ~13 months

// MessageMetrics implements domain.MessageMetrics using Redis INCR.
// Key format: lango:metrics:sent:{integrationID}:{YYYY-MM}
type MessageMetrics struct {
	rdb *goredis.Client
}

func NewMessageMetrics(rdb *goredis.Client) *MessageMetrics {
	return &MessageMetrics{rdb: rdb}
}

func (m *MessageMetrics) IncrementSent(ctx context.Context, integrationID uuid.UUID) error {
	key := sentKey(integrationID, time.Now().UTC())
	pipe := m.rdb.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, metricsTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func (m *MessageMetrics) GetMonthly(ctx context.Context, integrationID uuid.UUID, year int, month int) (int64, error) {
	t := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	key := sentKey(integrationID, t)
	val, err := m.rdb.Get(ctx, key).Int64()
	if err == goredis.Nil {
		return 0, nil
	}
	return val, err
}

func sentKey(integrationID uuid.UUID, t time.Time) string {
	return fmt.Sprintf("lango:metrics:sent:%s:%04d-%02d", integrationID, t.Year(), int(t.Month()))
}

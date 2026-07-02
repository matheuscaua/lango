-- +goose Up
-- +goose StatementBegin

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ──────────────────────────────────────────────────────────────────────────────
-- consumers
-- External services that talk to lango (e.g. "kituo-menyu-haraka"). Every
-- Integration belongs to exactly one consumer — see ADR 008 in the haraka repo
-- ("Identificação de consumidores e auditoria").
-- ──────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS consumers (
    id              UUID        NOT NULL DEFAULT uuid_generate_v4(),
    slug            TEXT        NOT NULL,
    api_key_hash    TEXT        NOT NULL,
    callback_url    TEXT        NOT NULL DEFAULT '',
    callback_secret TEXT        NOT NULL DEFAULT '',
    active          BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (id),
    CONSTRAINT consumers_slug_unique UNIQUE (slug),
    CONSTRAINT consumers_api_key_hash_unique UNIQUE (api_key_hash)
);

-- ──────────────────────────────────────────────────────────────────────────────
-- integrations
-- One WhatsApp number/channel, owned by exactly one consumer.
-- ──────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS integrations (
    id               UUID        NOT NULL DEFAULT uuid_generate_v4(),
    consumer_id      UUID        NOT NULL REFERENCES consumers(id) ON DELETE CASCADE,
    provider         TEXT        NOT NULL,
    phone_number_id  TEXT        NOT NULL,
    access_token     TEXT        NOT NULL,
    verify_token     TEXT        NOT NULL DEFAULT '',
    active           BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (id),
    CONSTRAINT integrations_provider_check CHECK (provider IN ('meta', 'evolution', 'twilio'))
);

CREATE INDEX IF NOT EXISTS idx_integrations_consumer_id
    ON integrations (consumer_id);

-- ──────────────────────────────────────────────────────────────────────────────
-- message_audit_entries
-- Append-only audit trail of every send attempt and every inbound webhook
-- receipt. Never updated except to move `status` forward through its
-- lifecycle (accepted -> sent/failed, received -> forwarded/forward_failed),
-- never deleted. See ADR 008 in the haraka repo.
-- ──────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS message_audit_entries (
    id             UUID        NOT NULL DEFAULT uuid_generate_v4(),
    consumer_id    UUID        NOT NULL REFERENCES consumers(id) ON DELETE CASCADE,
    integration_id UUID        NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    direction      TEXT        NOT NULL,
    provider       TEXT        NOT NULL,
    to_number      TEXT        NOT NULL DEFAULT '',
    from_number    TEXT        NOT NULL DEFAULT '',
    external_id    TEXT        NOT NULL DEFAULT '',
    status         TEXT        NOT NULL,
    error_reason   TEXT        NOT NULL DEFAULT '',
    correlation_id TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (id),
    CONSTRAINT audit_direction_check CHECK (direction IN ('inbound', 'outbound')),
    CONSTRAINT audit_status_check CHECK (
        status IN ('accepted', 'sent', 'failed', 'rejected', 'received', 'forwarded', 'forward_failed')
    )
);

CREATE INDEX IF NOT EXISTS idx_audit_consumer_created
    ON message_audit_entries (consumer_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_integration
    ON message_audit_entries (integration_id);

CREATE INDEX IF NOT EXISTS idx_audit_correlation_id
    ON message_audit_entries (correlation_id)
    WHERE correlation_id != '';

-- ──────────────────────────────────────────────────────────────────────────────
-- Row Level Security
-- Forces every query to filter by consumer_id — same convention haraka uses
-- for tenant_id. Application role must SET LOCAL app.consumer_id = '...'
-- before queries that should be consumer-scoped.
-- ──────────────────────────────────────────────────────────────────────────────
ALTER TABLE integrations           ENABLE ROW LEVEL SECURITY;
ALTER TABLE message_audit_entries  ENABLE ROW LEVEL SECURITY;

CREATE POLICY integrations_consumer_isolation ON integrations
    USING (consumer_id = current_setting('app.consumer_id')::UUID);

CREATE POLICY message_audit_entries_consumer_isolation ON message_audit_entries
    USING (consumer_id = current_setting('app.consumer_id')::UUID);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP POLICY IF EXISTS message_audit_entries_consumer_isolation ON message_audit_entries;
DROP POLICY IF EXISTS integrations_consumer_isolation          ON integrations;

DROP TABLE IF EXISTS message_audit_entries;
DROP TABLE IF EXISTS integrations;
DROP TABLE IF EXISTS consumers;

-- +goose StatementEnd

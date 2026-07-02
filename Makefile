BINARY_DIR := bin
DB_URL     ?= $(shell grep DATABASE_URL .env 2>/dev/null | cut -d= -f2-)
GOOSE      ?= $(shell go env GOPATH)/bin/goose

.PHONY: build test test-short lint run-api migrate-up migrate-down tidy setup

# ── Build ──────────────────────────────────────────────────────────────────────
build:
	go build -o $(BINARY_DIR)/api ./cmd/api

# ── Test ───────────────────────────────────────────────────────────────────────
test:
	go test -race -coverprofile=coverage.out ./...

test-short:
	go test -race -short ./...

# ── Lint ───────────────────────────────────────────────────────────────────────
lint:
	golangci-lint run ./...

# ── Run ────────────────────────────────────────────────────────────────────────
run-api: build
	./$(BINARY_DIR)/api

# ── Migrations ─────────────────────────────────────────────────────────────────
migrate-up:
	$(GOOSE) -dir migrations postgres "$(DB_URL)" up

migrate-down:
	$(GOOSE) -dir migrations postgres "$(DB_URL)" down

# ── Setup ─────────────────────────────────────────────────────────────────────
setup:
	go mod download
	go install github.com/pressly/goose/v3/cmd/goose@latest

tidy:
	go mod tidy

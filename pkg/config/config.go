package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// Server
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	// Database
	DatabaseURL string

	// Redis
	RedisURL string

	// Meta WhatsApp Cloud API — app-level secret, shared across every
	// integration that uses provider="meta" (Meta issues one App Secret per
	// app, not per phone number).
	WhatsAppAppSecret string

	// Evolution API (self-hosted) — base URL and admin API key, shared across
	// every integration that uses provider="evolution" (they're all instances
	// on the same host, authenticated with one global key). The per-integration
	// Integration.AccessToken is auto-filled with this same key on create —
	// existing message-send code paths keep reading it from there.
	EvolutionAPIURL string
	EvolutionAPIKey string

	// Base URL Evolution (running in Docker) uses to reach this service's
	// webhook — distinct from PublicWebhookBaseURL (a real public tunnel,
	// needed for Twilio's signature validation). Evolution and lango share the
	// same Docker host in local dev, so the container reaches the host process
	// via host.docker.internal instead of a public tunnel.
	EvolutionWebhookBaseURL string

	// Public HTTPS base URL for this service (e.g. a cloudflared tunnel) —
	// needed to verify Twilio webhook signatures, which are computed over the
	// exact URL Twilio was configured with.
	PublicWebhookBaseURL string

	// Observability
	LogLevel string
}

func Load() (*Config, error) {
	// Load .env if present (dev); in production env vars are set externally.
	_ = godotenv.Load()

	cfg := &Config{
		Port:         getEnv("PORT", "3100"),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		LogLevel:     getEnv("LOG_LEVEL", "info"),

		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisURL:    os.Getenv("REDIS_URL"),

		WhatsAppAppSecret: os.Getenv("WHATSAPP_APP_SECRET"),
		EvolutionAPIURL:   os.Getenv("EVOLUTION_API_URL"),
		EvolutionAPIKey:   getEnv("EVOLUTION_API_KEY", "lango-evolution-key"),

		PublicWebhookBaseURL: os.Getenv("PUBLIC_WEBHOOK_BASE_URL"),
	}
	cfg.EvolutionWebhookBaseURL = getEnv("EVOLUTION_WEBHOOK_BASE_URL", "http://host.docker.internal:"+cfg.Port)

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	var errs []error

	required := map[string]string{
		"DATABASE_URL": c.DatabaseURL,
		"REDIS_URL":    c.RedisURL,
	}

	for name, val := range required {
		if val == "" {
			errs = append(errs, fmt.Errorf("missing required env var: %s", name))
		}
	}

	return errors.Join(errs...)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

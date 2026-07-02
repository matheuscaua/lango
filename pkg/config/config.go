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

	// Evolution API (self-hosted) — base URL, shared across every integration
	// that uses provider="evolution" (they're all instances on the same host).
	EvolutionAPIURL string

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

		PublicWebhookBaseURL: os.Getenv("PUBLIC_WEBHOOK_BASE_URL"),
	}

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

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/kituomenyu/lango/internal/application"
	"github.com/kituomenyu/lango/internal/domain"
	"github.com/kituomenyu/lango/internal/infrastructure/evolutionapi"
	infrahttp "github.com/kituomenyu/lango/internal/infrastructure/http"
	"github.com/kituomenyu/lango/internal/infrastructure/postgres"
	infraredis "github.com/kituomenyu/lango/internal/infrastructure/redis"
	infratwilio "github.com/kituomenyu/lango/internal/infrastructure/twilio"
	"github.com/kituomenyu/lango/internal/infrastructure/whatsapp"
	"github.com/kituomenyu/lango/pkg/config"
	"github.com/kituomenyu/lango/pkg/database"
	pkgredis "github.com/kituomenyu/lango/pkg/redis"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	ctx := context.Background()

	// ── Infrastructure: database + cache ──────────────────────────────────────
	pool, err := database.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	rdb, err := pkgredis.New(cfg.RedisURL)
	if err != nil {
		slog.Error("failed to connect to redis", "err", err)
		os.Exit(1)
	}
	defer rdb.Close()

	// ── Infrastructure: repositories ──────────────────────────────────────────
	consumerRepo := postgres.NewConsumerRepository(pool)
	integrationRepo := postgres.NewIntegrationRepository(pool)
	auditRepo := postgres.NewAuditRepository(pool)
	msgMetrics := infraredis.NewMessageMetrics(rdb)

	// ── Infrastructure: providers ─────────────────────────────────────────────
	// Every provider is wired up front — a single lango instance serves
	// integrations of any provider, selected per-request by Integration.Provider.
	providers := map[string]domain.OutboundProvider{
		"meta":      whatsapp.NewProvider(integrationRepo).WithMetrics(msgMetrics),
		"evolution": evolutionapi.NewProvider(cfg.EvolutionAPIURL, integrationRepo).WithMetrics(msgMetrics),
		"twilio":    infratwilio.NewProvider(integrationRepo, rdb).WithMetrics(msgMetrics),
	}

	// ── Application: use cases ────────────────────────────────────────────────
	sendUC := application.NewSendMessageUseCase(integrationRepo, auditRepo, providers)
	forwardUC := application.NewForwardInboundUseCase(integrationRepo, consumerRepo, auditRepo)

	evolutionAdmin := evolutionapi.NewAdminClient(cfg.EvolutionAPIURL, cfg.EvolutionAPIKey)
	connectUC := application.NewConnectIntegrationUseCase(integrationRepo, evolutionAdmin, func(integrationID uuid.UUID) string {
		return fmt.Sprintf("%s/webhooks/evolution/%s", cfg.EvolutionWebhookBaseURL, integrationID)
	})
	disconnectUC := application.NewDisconnectIntegrationUseCase(integrationRepo, evolutionAdmin)

	// ── HTTP server ───────────────────────────────────────────────────────────
	app := fiber.New(fiber.Config{
		Network:      "tcp",
		AppName:      "lango",
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			msg := "internal server error"
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
				msg = e.Message
			}
			return c.Status(code).JSON(fiber.Map{"error": msg})
		},
	})

	app.Get("/health", func(c *fiber.Ctx) error {
		status := fiber.Map{"status": "ok"}
		code := fiber.StatusOK

		if err := database.Ping(c.Context(), pool); err != nil {
			status["postgres"] = "error"
			code = fiber.StatusServiceUnavailable
		} else {
			status["postgres"] = "ok"
		}

		if err := rdb.Ping(c.Context()).Err(); err != nil {
			status["redis"] = "error"
			code = fiber.StatusServiceUnavailable
		} else {
			status["redis"] = "ok"
		}

		return c.Status(code).JSON(status)
	})

	// ── Handlers + routes ─────────────────────────────────────────────────────
	webhookMeta := infrahttp.NewWebhookHandler(integrationRepo, forwardUC, cfg.WhatsAppAppSecret)
	webhookEvolution := infrahttp.NewWebhookEvolutionHandler(forwardUC, auditRepo, cfg.BotMessageMaxAgeSecs, cfg.BotAllowedPhones, cfg.BotEnableGroupReplies)
	webhookTwilio := infrahttp.NewWebhookTwilioHandler(integrationRepo, forwardUC, cfg.PublicWebhookBaseURL)
	integrationHandler := infrahttp.NewIntegrationHandler(integrationRepo, auditRepo, cfg.EvolutionAPIKey, connectUC, disconnectUC)
	messageHandler := infrahttp.NewMessageHandler(sendUC)
	auditHandler := infrahttp.NewAuditHandler(auditRepo)

	infrahttp.RegisterRoutes(app, consumerRepo,
		webhookMeta, webhookEvolution, webhookTwilio,
		integrationHandler, messageHandler, auditHandler,
	)

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("starting lango", 
			slog.String("port", cfg.Port),
			slog.Int("bot_max_age_seconds", cfg.BotMessageMaxAgeSecs),
			slog.Any("bot_allowed_phones", cfg.BotAllowedPhones),
		)
		if err := app.Listen(":" + cfg.Port); err != nil {
			slog.Error("server error", "err", err)
		}
	}()

	<-quit
	slog.Info("shutting down lango")
	if err := app.Shutdown(); err != nil {
		slog.Error("shutdown error", "err", err)
	}
}

// Package whatsapp provides an implementation of domain.OutboundProvider
// that delivers messages through the Meta WhatsApp Cloud API.
package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/kituomenyu/lango/internal/domain"
)

const (
	defaultBaseURL = "https://graph.facebook.com/v19.0"
	defaultTimeout = 10 * time.Second
)

// Provider implements domain.OutboundProvider using the WhatsApp Cloud API.
// It looks up per-integration credentials from IntegrationRepository.
type Provider struct {
	integrations domain.IntegrationRepository
	metrics      domain.MessageMetrics
	httpClient   *http.Client
	baseURL      string
}

// NewProvider creates a WhatsApp provider with production defaults.
func NewProvider(integrations domain.IntegrationRepository) *Provider {
	return &Provider{
		integrations: integrations,
		httpClient:   &http.Client{Timeout: defaultTimeout},
		baseURL:      defaultBaseURL,
	}
}

// WithMetrics attaches a MessageMetrics implementation for cost telemetry.
func (p *Provider) WithMetrics(m domain.MessageMetrics) *Provider {
	p.metrics = m
	return p
}

// WithBaseURL overrides the API base URL — used in tests to point at a mock server.
func (p *Provider) WithBaseURL(u string) *Provider {
	p.baseURL = u
	return p
}

// SendMessage delivers a text or template message to the customer's WhatsApp number.
func (p *Provider) SendMessage(ctx context.Context, integrationID uuid.UUID, phone string, msg *domain.Message) (string, error) {
	cfg, err := p.integrations.GetByID(ctx, integrationID)
	if err != nil {
		return "", fmt.Errorf("get integration %s: %w", integrationID, err)
	}

	body, err := buildSendBody(phone, msg)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/%s/messages", p.baseURL, cfg.PhoneNumberID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build whatsapp request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.AccessToken)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send whatsapp message: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("whatsapp API returned HTTP %d: %s", resp.StatusCode, respBody)
	}

	if p.metrics != nil {
		go func() {
			if err := p.metrics.IncrementSent(context.Background(), integrationID); err != nil {
				slog.Warn("metrics: failed to increment sent counter", "integration_id", integrationID, "err", err)
			}
		}()
	}

	// Meta Cloud API returns { messages: [{ id: "wamid..." }] }; that id is the
	// key its status webhooks (sent/delivered/read) reference. Best-effort parse.
	var sent struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(respBody, &sent)
	if len(sent.Messages) > 0 {
		return sent.Messages[0].ID, nil
	}
	return "", nil
}

// SendTemplate sends a WhatsApp template message (used for notifications outside 24h window).
func (p *Provider) SendTemplate(ctx context.Context, integrationID uuid.UUID, phone, templateName string, vars map[string]string) error {
	cfg, err := p.integrations.GetByID(ctx, integrationID)
	if err != nil {
		return fmt.Errorf("get integration %s: %w", integrationID, err)
	}

	components := make([]map[string]any, 0, len(vars))
	params := make([]map[string]string, 0, len(vars))
	for _, v := range vars {
		params = append(params, map[string]string{"type": "text", "text": v})
	}
	if len(params) > 0 {
		components = append(components, map[string]any{"type": "body", "parameters": params})
	}

	payload := map[string]any{
		"messaging_product": "whatsapp",
		"to":                phone,
		"type":              "template",
		"template": map[string]any{
			"name":       templateName,
			"language":   map[string]string{"code": "pt_BR"},
			"components": components,
		},
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/%s/messages", p.baseURL, cfg.PhoneNumberID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build template request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.AccessToken)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send template message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("whatsapp template API returned HTTP %d", resp.StatusCode)
	}

	if p.metrics != nil {
		go func() {
			if err := p.metrics.IncrementSent(context.Background(), integrationID); err != nil {
				slog.Warn("metrics: failed to increment sent counter", "integration_id", integrationID, "err", err)
			}
		}()
	}

	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func buildSendBody(phone string, msg *domain.Message) ([]byte, error) {
	var payload map[string]any

	switch msg.Type {
	case domain.TypeText:
		payload = map[string]any{
			"messaging_product": "whatsapp",
			"recipient_type":    "individual",
			"to":                phone,
			"type":              "text",
			"text": map[string]any{
				"preview_url": false,
				"body":        msg.Content,
			},
		}
	default:
		return nil, fmt.Errorf("unsupported outbound message type: %s", msg.Type)
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal whatsapp payload: %w", err)
	}
	return b, nil
}

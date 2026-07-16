// Package evolutionapi provides an implementation of domain.OutboundProvider
// that delivers messages through a self-hosted Evolution API instance (Baileys-based).
package evolutionapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kituomenyu/lango/internal/domain"
)

const defaultTimeout = 10 * time.Second

// Provider implements domain.OutboundProvider using the Evolution API.
// Integration.PhoneNumberID is the instance name; AccessToken is the API key.
type Provider struct {
	integrations domain.IntegrationRepository
	metrics      domain.MessageMetrics
	httpClient   *http.Client
	baseURL      string
}

func NewProvider(baseURL string, integrations domain.IntegrationRepository) *Provider {
	return &Provider{
		integrations: integrations,
		httpClient:   &http.Client{Timeout: defaultTimeout},
		baseURL:      baseURL,
	}
}

func (p *Provider) WithMetrics(m domain.MessageMetrics) *Provider {
	p.metrics = m
	return p
}

// WithBaseURL overrides the base URL — used in tests.
func (p *Provider) WithBaseURL(u string) *Provider {
	p.baseURL = u
	return p
}

func (p *Provider) SendMessage(ctx context.Context, integrationID uuid.UUID, phone string, msg *domain.Message) (string, error) {
	cfg, err := p.integrations.GetByID(ctx, integrationID)
	if err != nil {
		return "", fmt.Errorf("get integration %s: %w", integrationID, err)
	}

	var path string
	var payload map[string]any

	evolutionJID := strings.TrimPrefix(phone, "+")
	if !strings.Contains(evolutionJID, "@") {
		evolutionJID += "@s.whatsapp.net"
	}

	switch msg.Type {
	case domain.TypeText:
		path = "sendText"
		payload = map[string]any{"number": evolutionJID, "text": msg.Content}

	case domain.TypeInteractive:
		var list domain.ListMessage
		if err := json.Unmarshal([]byte(msg.Content), &list); err != nil {
			return "", fmt.Errorf("decode interactive list payload: %w", err)
		}
		path = "sendList"
		payload = map[string]any{
			"number":      evolutionJID,
			"title":       list.Title,
			"description": list.Description,
			"footerText":  list.Footer,
			"buttonText":  list.ButtonText,
			"sections":    list.Sections,
		}

	default:
		return "", fmt.Errorf("evolutionapi: unsupported message type %s", msg.Type)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal evolution payload: %w", err)
	}

	url := fmt.Sprintf("%s/message/%s/%s", p.baseURL, path, cfg.PhoneNumberID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build evolution request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", cfg.AccessToken)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send evolution message: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("evolution API returned HTTP %d: %s", resp.StatusCode, respBody)
	}

	if p.metrics != nil {
		go func() {
			if err := p.metrics.IncrementSent(context.Background(), integrationID); err != nil {
				slog.Warn("metrics: failed to increment sent counter", "integration_id", integrationID, "err", err)
			}
		}()
	}

	// Evolution echoes the sent message's Baileys key; key.id is what a later
	// MESSAGES_UPDATE (delivery/read ack) webhook references. Best-effort — an
	// unparseable body just means this send can't be correlated to a receipt.
	var sent struct {
		Key struct {
			ID string `json:"id"`
		} `json:"key"`
	}
	_ = json.Unmarshal(respBody, &sent)
	return sent.Key.ID, nil
}

// SendTemplate is not supported by Evolution API (no template approval flow).
// Falls back to sending plain text with the template name as a hint.
func (p *Provider) SendTemplate(ctx context.Context, integrationID uuid.UUID, phone, templateName string, vars map[string]string) error {
	cfg, err := p.integrations.GetByID(ctx, integrationID)
	if err != nil {
		return fmt.Errorf("get integration %s: %w", integrationID, err)
	}

	text := templateName
	for k, v := range vars {
		text += fmt.Sprintf("\n%s: %s", k, v)
	}

	evolutionJID := strings.TrimPrefix(phone, "+")
	if !strings.Contains(evolutionJID, "@") {
		evolutionJID += "@s.whatsapp.net"
	}

	payload, _ := json.Marshal(map[string]any{
		"number": evolutionJID,
		"text":   text,
	})

	url := fmt.Sprintf("%s/message/sendText/%s", p.baseURL, cfg.PhoneNumberID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build evolution template request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", cfg.AccessToken)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send evolution template: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("evolution API returned HTTP %d for template: %s", resp.StatusCode, respBody)
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

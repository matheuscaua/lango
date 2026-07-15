// Package twilio provides an implementation of domain.OutboundProvider that
// delivers messages through Twilio's WhatsApp API. Unlike evolutionapi (which
// talks to Baileys, an unofficial reverse-engineered WhatsApp Web protocol),
// Twilio is a Business Solution Provider sitting in front of Meta's official
// WhatsApp Business Platform — so interactive messages (lists/buttons) are
// fully and reliably supported here.
package twilio

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kituomenyu/lango/internal/domain"
	goredis "github.com/redis/go-redis/v9"
)

const (
	defaultTimeout      = 10 * time.Second
	defaultTemplateTTL  = 24 * time.Hour // content templates change rarely; keyed by content hash anyway
	messagesAPIBaseURL  = "https://api.twilio.com/2010-04-01"
	contentAPIBaseURL   = "https://content.twilio.com/v1"
	contentTemplateType = "twilio/list-picker"
	contentCachePrefix  = "twilio:content:"
)

// Provider implements domain.OutboundProvider using Twilio's WhatsApp API.
// Integration.PhoneNumberID is the Twilio "From" number (E.164, no
// "whatsapp:" prefix); AccessToken is the Auth Token; VerifyToken is
// repurposed to hold the Account SID (unused by the other providers).
type Provider struct {
	integrations domain.IntegrationRepository
	metrics      domain.MessageMetrics
	httpClient   *http.Client
	redis        *goredis.Client // nil disables content-template caching
}

// NewProvider creates a Twilio provider. rdb may be nil (each interactive
// send then creates a fresh Content Template instead of reusing a cached one).
func NewProvider(integrations domain.IntegrationRepository, rdb *goredis.Client) *Provider {
	return &Provider{
		integrations: integrations,
		httpClient:   &http.Client{Timeout: defaultTimeout},
		redis:        rdb,
	}
}

func (p *Provider) WithMetrics(m domain.MessageMetrics) *Provider {
	p.metrics = m
	return p
}

func (p *Provider) SendMessage(ctx context.Context, integrationID uuid.UUID, phone string, msg *domain.Message) (string, error) {
	cfg, err := p.integrations.GetByID(ctx, integrationID)
	if err != nil {
		return "", fmt.Errorf("get integration %s: %w", integrationID, err)
	}
	accountSID := cfg.VerifyToken
	authToken := cfg.AccessToken

	form := url.Values{}
	form.Set("To", toWhatsAppAddress(phone))
	form.Set("From", toWhatsAppAddress(cfg.PhoneNumberID))

	switch msg.Type {
	case domain.TypeText:
		form.Set("Body", msg.Content)

	case domain.TypeInteractive:
		var list domain.ListMessage
		if err := json.Unmarshal([]byte(msg.Content), &list); err != nil {
			return "", fmt.Errorf("decode interactive list payload: %w", err)
		}
		contentSID, err := p.getOrCreateContentTemplate(ctx, accountSID, authToken, list)
		if err != nil {
			return "", fmt.Errorf("resolve content template: %w", err)
		}
		form.Set("ContentSid", contentSID)

	default:
		return "", fmt.Errorf("twilio: unsupported message type %s", msg.Type)
	}

	messagesURL := fmt.Sprintf("%s/Accounts/%s/Messages.json", messagesAPIBaseURL, accountSID)
	body := strings.NewReader(form.Encode())
	respBody, err := p.doRequest(ctx, messagesURL, accountSID, authToken, body, "application/x-www-form-urlencoded")
	if err != nil {
		return "", fmt.Errorf("send twilio message: %w", err)
	}

	if p.metrics != nil {
		go func() {
			if err := p.metrics.IncrementSent(context.Background(), integrationID); err != nil {
				slog.Warn("metrics: failed to increment sent counter", "integration_id", integrationID, "err", err)
			}
		}()
	}

	// Twilio returns { "sid": "SM..." }; that SID is what its status callbacks
	// (delivered/read) reference. Best-effort parse.
	var sent struct {
		SID string `json:"sid"`
	}
	_ = json.Unmarshal(respBody, &sent)
	return sent.SID, nil
}

// SendTemplate is a lightweight fallback (matches evolutionapi's behavior):
// Twilio's real template mechanism is the Content API used above for
// interactive messages; there's no separate named-template flow wired up yet,
// so this just sends the name + vars as plain text.
func (p *Provider) SendTemplate(ctx context.Context, integrationID uuid.UUID, phone, templateName string, vars map[string]string) error {
	text := templateName
	for k, v := range vars {
		text += fmt.Sprintf("\n%s: %s", k, v)
	}
	_, err := p.SendMessage(ctx, integrationID, phone, &domain.Message{Type: domain.TypeText, Content: text})
	return err
}

// getOrCreateContentTemplate resolves a Twilio Content Template SID for the
// given list, reusing a cached one (keyed by a hash of the list's content) or
// creating a fresh "twilio/list-picker" template via the Content API. All
// Sections are flattened into a single items array — WhatsApp's list message
// supports up to 10 rows total.
func (p *Provider) getOrCreateContentTemplate(ctx context.Context, accountSID, authToken string, list domain.ListMessage) (string, error) {
	hash := hashListMessage(list)
	cacheKey := contentCachePrefix + hash

	if p.redis != nil {
		if sid, err := p.redis.Get(ctx, cacheKey).Result(); err == nil && sid != "" {
			return sid, nil
		}
	}

	items := make([]map[string]any, 0)
	for _, sec := range list.Sections {
		for _, row := range sec.Rows {
			// WhatsApp caps list row titles at 24 chars and descriptions at 72
			// (Twilio enforces the same limits). The consumer's own copy is
			// written to fit, but truncate defensively rather than let the
			// send fail on data lango doesn't control.
			item := map[string]any{"item": truncate(row.Title, 24), "id": row.RowID}
			if row.Description != "" {
				item["description"] = truncate(row.Description, 72)
			}
			items = append(items, item)
		}
	}

	payload := map[string]any{
		"friendly_name": "lango_" + hash[:16],
		"language":      "pt_BR",
		"types": map[string]any{
			contentTemplateType: map[string]any{
				"body":   list.Description,
				"button": truncate(list.ButtonText, 20),
				"items":  items,
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal content template: %w", err)
	}

	respBody, err := p.doRequest(ctx, contentAPIBaseURL+"/Content", accountSID, authToken, bytes.NewReader(body), "application/json")
	if err != nil {
		return "", fmt.Errorf("create content template: %w", err)
	}

	var created struct {
		SID string `json:"sid"`
	}
	if err := json.Unmarshal(respBody, &created); err != nil || created.SID == "" {
		return "", fmt.Errorf("unexpected content template response: %s", respBody)
	}

	if p.redis != nil {
		_ = p.redis.Set(ctx, cacheKey, created.SID, defaultTemplateTTL).Err()
	}

	return created.SID, nil
}

// doRequest POSTs to the Twilio API with HTTP Basic auth (Account SID / Auth
// Token) and returns the response body, or an error for non-2xx responses.
func (p *Provider) doRequest(ctx context.Context, reqURL, accountSID, authToken string, body io.Reader, contentType string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.SetBasicAuth(accountSID, authToken)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("twilio API returned HTTP %d: %s", resp.StatusCode, respBody)
	}
	return respBody, nil
}

// truncate shortens s to at most n runes, so multi-byte characters (emoji,
// accents) aren't split mid-codepoint.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func toWhatsAppAddress(phone string) string {
	phone = strings.TrimPrefix(phone, "whatsapp:")
	phone = strings.TrimPrefix(phone, "+")
	return "whatsapp:+" + phone
}

func hashListMessage(list domain.ListMessage) string {
	data, _ := json.Marshal(list)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

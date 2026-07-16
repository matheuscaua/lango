package evolutionapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kituomenyu/lango/internal/domain"
)

const adminTimeout = 10 * time.Second

// AdminClient wraps the Evolution API's instance-management endpoints (as
// opposed to Provider, which only sends messages through an already-connected
// instance). Used exclusively by the QR-code connect flow.
type AdminClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func NewAdminClient(baseURL, apiKey string) *AdminClient {
	return &AdminClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: adminTimeout},
	}
}

func (c *AdminClient) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal request body: %w", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, r)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("call evolution api: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return respBody, resp.StatusCode, nil
}

// CreateInstance is idempotent — a 409 (already exists) is treated as success.
func (c *AdminClient) CreateInstance(ctx context.Context, instanceName string) error {
	payload := map[string]any{
		"instanceName": instanceName,
		"integration":  "WHATSAPP-BAILEYS",
	}
	body, status, err := c.do(ctx, http.MethodPost, "/instance/create", payload)
	if err != nil {
		return err
	}
	// "Already exists" is a success case for us (idempotent connect/reconnect
	// flow), but Evolution v2.3.6 reports it inconsistently: 409 Conflict on
	// some builds, 403 Forbidden with a `{"message":["... already in use."]}`
	// body on this one (empirically confirmed against evoapicloud/v2.3.6) —
	// so match on the message instead of trusting the status code alone.
	if status == http.StatusConflict || (status == http.StatusForbidden && bytes.Contains(body, []byte("already in use"))) {
		return nil
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("evolution create instance: HTTP %d: %s", status, string(body))
	}
	return nil
}

// SetWebhook is idempotent — safe to call on every connect attempt.
func (c *AdminClient) SetWebhook(ctx context.Context, instanceName, webhookURL string) error {
	payload := map[string]any{
		"webhook": map[string]any{
			"url": webhookURL,
			// MESSAGES_UPSERT = inbound customer messages; MESSAGES_UPDATE =
			// delivery/read acks for messages we sent (see webhook handler).
			"events":  []string{"MESSAGES_UPSERT", "MESSAGES_UPDATE"},
			"enabled": true,
		},
	}
	body, status, err := c.do(ctx, http.MethodPost, "/webhook/set/"+instanceName, payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("evolution set webhook: HTTP %d: %s", status, string(body))
	}
	return nil
}

// ConnectionState returns Evolution's raw state string (e.g. "open",
// "connecting", "close").
func (c *AdminClient) ConnectionState(ctx context.Context, instanceName string) (string, error) {
	body, status, err := c.do(ctx, http.MethodGet, "/instance/connectionState/"+instanceName, nil)
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("evolution connection state: HTTP %d: %s", status, string(body))
	}
	var resp struct {
		Instance struct {
			State string `json:"state"`
		} `json:"instance"`
		State string `json:"state"` // some versions respond top-level
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("decode connection state: %w", err)
	}
	if resp.Instance.State != "" {
		return resp.Instance.State, nil
	}
	return resp.State, nil
}

// LogoutInstance ends the WhatsApp session for an instance (Evolution
// `DELETE /instance/logout/:name`), unpairing the linked phone. Idempotent: a
// 404 (instance gone) or a "not connected" 400/401 is treated as success —
// either way the number ends up disconnected.
func (c *AdminClient) LogoutInstance(ctx context.Context, instanceName string) error {
	body, status, err := c.do(ctx, http.MethodDelete, "/instance/logout/"+instanceName, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound || status == http.StatusBadRequest || status == http.StatusUnauthorized {
		return nil // already logged out / not connected — desired end state reached
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("evolution logout instance: HTTP %d: %s", status, string(body))
	}
	return nil
}

// DeleteInstance removes an instance entirely (Evolution
// `DELETE /instance/delete/:name`). Idempotent: a 404 means it's already gone.
// Evolution requires an instance to be logged out before deletion, so callers
// should LogoutInstance first.
func (c *AdminClient) DeleteInstance(ctx context.Context, instanceName string) error {
	body, status, err := c.do(ctx, http.MethodDelete, "/instance/delete/"+instanceName, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return nil
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("evolution delete instance: HTTP %d: %s", status, string(body))
	}
	return nil
}

// GetQR fetches the current QR code as a base64-encoded PNG (data URI prefix
// stripped). Baileys QR codes expire after ~20-30s — callers must re-fetch
// periodically while polling for connection.
func (c *AdminClient) GetQR(ctx context.Context, instanceName string) (string, error) {
	body, status, err := c.do(ctx, http.MethodGet, "/instance/connect/"+instanceName, nil)
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("evolution get QR: HTTP %d: %s", status, string(body))
	}
	var resp struct {
		Base64 string `json:"base64"` // data URI: "data:image/png;base64,<data>"
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("decode QR response: %w", err)
	}
	if resp.Base64 == "" {
		return "", domain.ErrConnectQRNotAvailable
	}
	data := resp.Base64
	if i := strings.Index(data, ","); strings.HasPrefix(data, "data:") && i != -1 {
		data = data[i+1:]
	}
	return data, nil
}

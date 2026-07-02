package domain

import "time"

// InboundEvent is the canonical shape lango forwards to a consumer's callback
// URL for every inbound webhook message, regardless of which provider it came
// from (Meta, Evolution, Twilio all normalize into this same struct).
type InboundEvent struct {
	IntegrationID string    `json:"integration_id"`
	From          string    `json:"from"` // E.164
	Content       string    `json:"content"`
	ButtonPayload string    `json:"button_payload,omitempty"` // opaque row/button ID, see ListRow
	ExternalID    string    `json:"external_id"`
	ReceivedAt    time.Time `json:"received_at"`
}

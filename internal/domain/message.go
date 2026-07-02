package domain

type MessageType string

const (
	TypeText        MessageType = "TEXT"
	TypeInteractive MessageType = "INTERACTIVE"
	TypeTemplate    MessageType = "TEMPLATE"
	TypeMedia       MessageType = "MEDIA"
)

// Message is the payload lango dispatches to a provider. It carries no
// business meaning — Content is opaque text, and for TypeInteractive it is a
// JSON-encoded ListMessage whose row IDs are generated and interpreted only
// by the consumer (e.g. haraka), never by lango.
type Message struct {
	Type    MessageType
	Content string
}

// ── Interactive message payloads ───────────────────────────────────────────────
//
// A ListMessage is WhatsApp's native tap-to-select menu. It travels as a JSON
// string in Message.Content when Type is TypeInteractive; a provider that
// supports it (Evolution, Twilio) decodes Content back into a ListMessage to
// build its own request body.

// ListRow is one selectable option inside a ListSection. RowID is echoed back
// by the provider's webhook when the customer taps it. lango treats it as an
// opaque string — it must never generate, parse, or attach meaning to a RowID;
// that is exclusively the consumer's responsibility.
type ListRow struct {
	RowID       string `json:"rowId"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// ListSection groups rows under a heading inside a ListMessage.
type ListSection struct {
	Title string    `json:"title"`
	Rows  []ListRow `json:"rows"`
}

// ListMessage is a WhatsApp List Message: a short body (Title/Description)
// plus a button that opens a picker grouped into sections.
type ListMessage struct {
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Footer      string        `json:"footer"`
	ButtonText  string        `json:"buttonText"`
	Sections    []ListSection `json:"sections"`
}

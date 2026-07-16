package http

import "testing"

func TestResolveInboundPhone(t *testing.T) {
	tests := []struct {
		name      string
		key       evolutionKey
		wantPhone string
		wantOK    bool
	}{
		{
			name:      "plain phone JID",
			key:       evolutionKey{RemoteJid: "553192998146@s.whatsapp.net"},
			wantPhone: "553192998146",
			wantOK:    true,
		},
		{
			name:      "legacy @c.us JID",
			key:       evolutionKey{RemoteJid: "553192998146@c.us"},
			wantPhone: "553192998146",
			wantOK:    true,
		},
		{
			// The bug this fixes: LID addressing carries an opaque id in
			// remoteJid and the real number in remoteJidAlt.
			name: "LID uses remoteJidAlt for the real number",
			key: evolutionKey{
				RemoteJid:      "10235058081845@lid",
				RemoteJidAlt:   "553192998146@s.whatsapp.net",
				AddressingMode: "lid",
			},
			wantPhone: "553192998146",
			wantOK:    true,
		},
		{
			name: "LID detected by suffix even without addressingMode",
			key: evolutionKey{
				RemoteJid:    "10235058081845@lid",
				RemoteJidAlt: "553192998146@s.whatsapp.net",
			},
			wantPhone: "553192998146",
			wantOK:    true,
		},
		{
			name:   "LID without an alternate number is unresolvable → skip",
			key:    evolutionKey{RemoteJid: "146170689130567@lid", AddressingMode: "lid"},
			wantOK: false,
		},
		{
			name:   "group chat skipped",
			key:    evolutionKey{RemoteJid: "123456789@g.us"},
			wantOK: false,
		},
		{
			name:   "broadcast skipped",
			key:    evolutionKey{RemoteJid: "status@broadcast"},
			wantOK: false,
		},
		{
			name:   "newsletter skipped",
			key:    evolutionKey{RemoteJid: "abc@newsletter"},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			phone, ok := resolveInboundPhone(tt.key, false)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (phone=%q)", ok, tt.wantOK, phone)
			}
			if ok && phone != tt.wantPhone {
				t.Errorf("phone = %q, want %q", phone, tt.wantPhone)
			}
		})
	}

	t.Run("group chat allowed when flag is enabled", func(t *testing.T) {
		key := evolutionKey{RemoteJid: "123456789@g.us"}
		// When flag is true, we should get the phone back and ok = true
		phone, ok := resolveInboundPhone(key, true)
		if !ok {
			t.Fatalf("expected ok=true for group chat with flag enabled")
		}
		if phone != "123456789@g.us" {
			t.Errorf("expected phone='123456789@g.us', got %q", phone)
		}
	})
}

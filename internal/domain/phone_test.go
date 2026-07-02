package domain_test

import (
	"testing"

	"github.com/kituomenyu/lango/internal/domain"
)

func TestIsE164(t *testing.T) {
	tests := []struct {
		phone string
		want  bool
	}{
		{"+5511999990000", true},
		{"+14155238886", true},
		{"5511999990000", false},  // missing +
		{"+0511999990000", false}, // leading zero after +
		{"", false},
		{"not-a-phone", false},
		{"+55 11 99999-0000", false}, // formatting characters not allowed
	}

	for _, tt := range tests {
		if got := domain.IsE164(tt.phone); got != tt.want {
			t.Errorf("IsE164(%q) = %v, want %v", tt.phone, got, tt.want)
		}
	}
}

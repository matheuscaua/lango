package auth_test

import (
	"testing"

	"github.com/kituomenyu/lango/pkg/auth"
)

func TestGenerateAPIKey_Is64HexChars(t *testing.T) {
	key, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 64 {
		t.Errorf("key length = %d, want 64", len(key))
	}
}

func TestGenerateAPIKey_IsUnique(t *testing.T) {
	a, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	b, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("two generated keys should not collide")
	}
}

func TestHashAPIKey_IsDeterministic(t *testing.T) {
	key := "some-fixed-key-for-testing"
	if auth.HashAPIKey(key) != auth.HashAPIKey(key) {
		t.Error("hashing the same key twice should produce the same hash")
	}
}

func TestHashAPIKey_DiffersFromInput(t *testing.T) {
	key := "some-fixed-key-for-testing"
	if auth.HashAPIKey(key) == key {
		t.Error("hash must not equal the raw key")
	}
}

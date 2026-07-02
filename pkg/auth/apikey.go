// Package auth provides API key generation and hashing for consumer authentication.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// GenerateAPIKey returns a fresh, cryptographically random API key (64 hex
// chars). It is shown to the operator exactly once, at consumer creation —
// lango only ever stores its hash (see HashAPIKey).
func GenerateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// HashAPIKey returns the SHA-256 hash of an API key, hex-encoded. Lookups
// hash the incoming key and compare hashes — the raw key is never persisted.
func HashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

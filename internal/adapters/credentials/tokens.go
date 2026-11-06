package credentials

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
)

// tokenBytes is the session token entropy (ADR 0032 section 3).
const tokenBytes = 32

// Tokens creates session tokens and hashes them for storage; the database keeps only the hash.
// The zero value draws from crypto/rand.
type Tokens struct {
	random io.Reader
}

// NewTokens returns Tokens drawing from crypto/rand.
func NewTokens() Tokens {
	return Tokens{random: rand.Reader}
}

// New returns a fresh token (32 random bytes, base64url without padding) and its HashToken.
func (t Tokens) New() (token, hash string, err error) {
	raw := make([]byte, tokenBytes)
	source := t.random
	if source == nil {
		source = rand.Reader
	}
	if _, err := io.ReadFull(source, raw); err != nil {
		return "", "", fmt.Errorf("new session token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, t.HashToken(token), nil
}

// HashToken returns the hex SHA-256 of token. A plain hash suffices because the token carries
// 256 bits of entropy; a slow hash would only add work to every request.
func (Tokens) HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

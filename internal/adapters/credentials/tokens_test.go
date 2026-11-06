package credentials

import (
	"encoding/base64"
	"testing"
)

func TestTokensNewReturnsUniqueURLSafeTokensWithMatchingHash(t *testing.T) {
	t.Parallel()
	const draws = 1000
	tokens := NewTokens()
	seen := make(map[string]struct{}, draws)
	for range draws {
		token, hash, err := tokens.New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		raw, err := base64.RawURLEncoding.DecodeString(token)
		if err != nil || len(raw) != tokenBytes {
			t.Fatalf("token %q: %d bytes, err %v; want %d raw base64url bytes", token, len(raw), err, tokenBytes)
		}
		if hash != tokens.HashToken(token) {
			t.Fatalf("New hash %q differs from HashToken %q", hash, tokens.HashToken(token))
		}
		if _, duplicate := seen[token]; duplicate {
			t.Fatalf("token %q drawn twice", token)
		}
		seen[token] = struct{}{}
	}
}

func TestHashTokenMatchesSHA256Vectors(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"":    "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"abc": "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	}
	for token, want := range tests {
		if got := NewTokens().HashToken(token); got != want {
			t.Errorf("HashToken(%q) = %q, want %q", token, got, want)
		}
	}
}

func TestTokensNewReportsRandomSourceFailure(t *testing.T) {
	t.Parallel()
	tokens := Tokens{random: failingReader{}}
	if _, _, err := tokens.New(); err == nil {
		t.Fatal("New with a failing random source returned no error")
	}
}

func TestZeroValueTokensDrawFromCryptoRand(t *testing.T) {
	t.Parallel()
	if token, _, err := (Tokens{}).New(); err != nil || token == "" {
		t.Fatalf("zero-value New = (%q, %v), want a token", token, err)
	}
}

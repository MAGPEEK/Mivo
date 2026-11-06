// Package credentials hashes passwords and PINs with argon2id and creates session tokens
// (ADR 0032 sections 2 and 3). It implements the hashing and token ports of app/auth and
// app/users.
package credentials

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
)

// The production cost is the OWASP argon2id minimum (19 MiB, 2 passes, 1 lane): Mivo runs on
// 1 GB NAS devices, and with maxConcurrentHashes the peak stays below 40 MiB.
const (
	productionMemoryKiB = 19 * 1024
	productionPasses    = 2
	productionLanes     = 1
	saltLength          = 16
	keyLength           = 32
	// maxConcurrentHashes bounds memory use during a burst of sign-ins.
	maxConcurrentHashes = 2
)

// parameters is one argon2id cost setting.
type parameters struct {
	memoryKiB uint32
	passes    uint32
	lanes     uint8
}

// weakerThan reports whether any cost of p is below the matching cost of other.
func (p parameters) weakerThan(other parameters) bool {
	return p.memoryKiB < other.memoryKiB || p.passes < other.passes || p.lanes < other.lanes
}

// Hasher hashes and verifies secrets with argon2id. It is safe for concurrent use; at most
// maxConcurrentHashes operations compute at once, the others wait for a slot.
type Hasher struct {
	params parameters
	slots  chan struct{}
	random io.Reader
}

// NewHasher returns a Hasher with the production parameters.
func NewHasher() *Hasher {
	return newHasher(parameters{
		memoryKiB: productionMemoryKiB,
		passes:    productionPasses,
		lanes:     productionLanes,
	})
}

// newHasher lets tests lower the cost.
func newHasher(params parameters) *Hasher {
	return &Hasher{params: params, slots: make(chan struct{}, maxConcurrentHashes), random: rand.Reader}
}

// Hash returns the PHC string of secret with a fresh random salt.
func (h *Hasher) Hash(ctx context.Context, secret string) (string, error) {
	salt := make([]byte, saltLength)
	if _, err := io.ReadFull(h.random, salt); err != nil {
		return "", fmt.Errorf("hash secret: read salt: %w", err)
	}
	key, err := h.derive(ctx, secret, derivation{params: h.params, salt: salt, keyLength: keyLength})
	if err != nil {
		return "", fmt.Errorf("hash secret: %w", err)
	}
	return encodePHC(h.params, salt, key), nil
}

// Verify reports whether secret matches the PHC string encoded. needsRehash is true only for
// a match whose stored parameters are weaker than the current ones, so the caller can store
// a fresh Hash. A malformed encoding wraps domain.ErrInvalid.
func (h *Hasher) Verify(ctx context.Context, secret, encoded string) (ok, needsRehash bool, err error) {
	stored, err := parsePHC(encoded)
	if err != nil {
		return false, false, fmt.Errorf("verify secret: %w", err)
	}
	key, err := h.derive(ctx, secret, derivation{params: stored.params, salt: stored.salt, keyLength: len(stored.key)})
	if err != nil {
		return false, false, fmt.Errorf("verify secret: %w", err)
	}
	if subtle.ConstantTimeCompare(key, stored.key) != 1 {
		return false, false, nil
	}
	weaker := stored.params.weakerThan(h.params) || len(stored.key) < keyLength || len(stored.salt) < saltLength
	return true, weaker, nil
}

// DummyVerify spends the work of one Verify with the current parameters and discards the
// result. Sign-in calls it for unknown account names so timing does not reveal them.
func (h *Hasher) DummyVerify(ctx context.Context, secret string) error {
	dummySalt := make([]byte, saltLength)
	key, err := h.derive(ctx, secret, derivation{params: h.params, salt: dummySalt, keyLength: keyLength})
	if err != nil {
		return fmt.Errorf("dummy verify: %w", err)
	}
	// Compare like Verify does so both paths cost the same.
	subtle.ConstantTimeCompare(key, make([]byte, keyLength))
	return nil
}

// derivation is the input of one argon2id computation besides the secret.
type derivation struct {
	params    parameters
	salt      []byte
	keyLength int
}

// derive computes the argon2id key once a slot is free.
func (h *Hasher) derive(ctx context.Context, secret string, input derivation) ([]byte, error) {
	if err := h.acquire(ctx); err != nil {
		return nil, err
	}
	defer h.release()
	params := input.params
	//nolint:gosec // keyLength is bounded by maximumKeyLength or keyLength
	return argon2.IDKey([]byte(secret), input.salt, params.passes, params.memoryKiB, params.lanes, uint32(input.keyLength)), nil
}

func (h *Hasher) acquire(ctx context.Context) error {
	// Check first: select picks randomly when both a slot and Done are ready.
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("wait for hash slot: %w", err)
	}
	select {
	case h.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for hash slot: %w", ctx.Err())
	}
}

func (h *Hasher) release() {
	<-h.slots
}

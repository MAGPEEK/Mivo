package credentials

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
)

// testParameters keep the argon2id cost tiny so the suite stays fast under -race.
func testParameters() parameters {
	return parameters{memoryKiB: 64, passes: 1, lanes: 1}
}

func newTestHasher() *Hasher {
	return newHasher(testParameters())
}

func TestHashAndVerifyRoundTripWithProductionParameters(t *testing.T) {
	t.Parallel()
	hasher := NewHasher()

	encoded, err := hasher.Hash(t.Context(), "correct horse")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("encoded %q lacks the production PHC prefix", encoded)
	}
	assertSaltAndKeyLengths(t, encoded)

	ok, needsRehash, err := hasher.Verify(t.Context(), "correct horse", encoded)
	if err != nil || !ok || needsRehash {
		t.Fatalf("Verify = (%v, %v, %v), want (true, false, nil)", ok, needsRehash, err)
	}
}

func assertSaltAndKeyLengths(t *testing.T, encoded string) {
	t.Helper()
	fields := strings.Split(encoded, "$")
	salt, err := base64.RawStdEncoding.DecodeString(fields[4])
	if err != nil || len(salt) != saltLength {
		t.Fatalf("salt %q: length %d, err %v", fields[4], len(salt), err)
	}
	key, err := base64.RawStdEncoding.DecodeString(fields[5])
	if err != nil || len(key) != keyLength {
		t.Fatalf("key %q: length %d, err %v", fields[5], len(key), err)
	}
}

func TestHashUsesAFreshSaltEveryTime(t *testing.T) {
	t.Parallel()
	hasher := newTestHasher()
	first, err := hasher.Hash(t.Context(), "1234")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	second, err := hasher.Hash(t.Context(), "1234")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if first == second {
		t.Fatalf("two hashes of the same secret are equal: %q", first)
	}
}

func TestVerifyRejectsWrongAndTamperedInput(t *testing.T) {
	t.Parallel()
	hasher := newTestHasher()
	encoded, err := hasher.Hash(t.Context(), "secret")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	fields := strings.Split(encoded, "$")
	otherSalt := base64.RawStdEncoding.EncodeToString([]byte("0123456789abcdef"))
	otherKey := base64.RawStdEncoding.EncodeToString(make([]byte, keyLength))

	tests := []struct {
		name    string
		secret  string
		encoded string
	}{
		{name: "wrong secret", secret: "Secret", encoded: encoded},
		{name: "empty secret", secret: "", encoded: encoded},
		{name: "tampered salt", secret: "secret", encoded: replaceField(fields, 4, otherSalt)},
		{name: "tampered hash", secret: "secret", encoded: replaceField(fields, 5, otherKey)},
		{name: "raised passes", secret: "secret", encoded: replaceField(fields, 3, "m=64,t=2,p=1")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ok, needsRehash, err := hasher.Verify(t.Context(), test.secret, test.encoded)
			if err != nil || ok || needsRehash {
				t.Fatalf("Verify = (%v, %v, %v), want (false, false, nil)", ok, needsRehash, err)
			}
		})
	}
}

func replaceField(fields []string, index int, value string) string {
	changed := append([]string(nil), fields...)
	changed[index] = value
	return strings.Join(changed, "$")
}

func TestVerifyReportsMalformedEncodingsAsInvalid(t *testing.T) {
	t.Parallel()
	const salt = "MDEyMzQ1Njc4OWFiY2RlZg"
	const key = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	tests := map[string]string{
		"empty":               "",
		"bcrypt":              "$2a$10$abcdefghijklmnopqrstuv",
		"argon2i":             "$argon2i$v=19$m=64,t=1,p=1$" + salt + "$" + key,
		"too few fields":      "$argon2id$v=19$m=64,t=1,p=1$" + salt,
		"no leading dollar":   "argon2id$v=19$m=64,t=1,p=1$" + salt + "$" + key + "$",
		"old version":         "$argon2id$v=16$m=64,t=1,p=1$" + salt + "$" + key,
		"missing parameter":   "$argon2id$v=19$m=64,t=1$" + salt + "$" + key,
		"reordered":           "$argon2id$v=19$t=1,m=64,p=1$" + salt + "$" + key,
		"non-numeric":         "$argon2id$v=19$m=x,t=1,p=1$" + salt + "$" + key,
		"zero passes":         "$argon2id$v=19$m=64,t=0,p=1$" + salt + "$" + key,
		"zero lanes":          "$argon2id$v=19$m=64,t=1,p=0$" + salt + "$" + key,
		"memory below lanes":  "$argon2id$v=19$m=4,t=1,p=1$" + salt + "$" + key,
		"memory too large":    "$argon2id$v=19$m=999999999,t=1,p=1$" + salt + "$" + key,
		"too many passes":     "$argon2id$v=19$m=64,t=1000,p=1$" + salt + "$" + key,
		"too many lanes":      "$argon2id$v=19$m=64,t=1,p=300$" + salt + "$" + key,
		"salt not base64":     "$argon2id$v=19$m=64,t=1,p=1$!!!!$" + key,
		"salt too short":      "$argon2id$v=19$m=64,t=1,p=1$MDEy$" + key,
		"hash not base64":     "$argon2id$v=19$m=64,t=1,p=1$" + salt + "$!!!!",
		"hash too short":      "$argon2id$v=19$m=64,t=1,p=1$" + salt + "$AAAA",
		"padded salt":         "$argon2id$v=19$m=64,t=1,p=1$" + salt + "==$" + key,
		"empty parameter set": "$argon2id$v=19$$" + salt + "$" + key,
	}
	hasher := newTestHasher()
	for name, encoded := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ok, _, err := hasher.Verify(t.Context(), "secret", encoded)
			if !errors.Is(err, domain.ErrInvalid) || ok {
				t.Fatalf("Verify(%q) = (%v, %v), want ErrInvalid", encoded, ok, err)
			}
		})
	}
}

func TestVerifyFlagsWeakerStoredParametersForRehash(t *testing.T) {
	t.Parallel()
	weak := parameters{memoryKiB: 32, passes: 1, lanes: 1}
	tests := []struct {
		name    string
		stored  parameters
		current parameters
		want    bool
	}{
		{name: "same", stored: weak, current: weak, want: false},
		{name: "less memory", stored: weak, current: parameters{memoryKiB: 64, passes: 1, lanes: 1}, want: true},
		{name: "fewer passes", stored: weak, current: parameters{memoryKiB: 32, passes: 2, lanes: 1}, want: true},
		{name: "fewer lanes", stored: weak, current: parameters{memoryKiB: 32, passes: 1, lanes: 2}, want: true},
		{name: "stronger stored", stored: parameters{memoryKiB: 64, passes: 2, lanes: 1}, current: weak, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			encoded, err := newHasher(test.stored).Hash(t.Context(), "secret")
			if err != nil {
				t.Fatalf("Hash: %v", err)
			}
			ok, needsRehash, err := newHasher(test.current).Verify(t.Context(), "secret", encoded)
			if err != nil || !ok || needsRehash != test.want {
				t.Fatalf("Verify = (%v, %v, %v), want (true, %v, nil)", ok, needsRehash, err, test.want)
			}
		})
	}
}

func TestVerifyNeverRequestsRehashForAWrongSecret(t *testing.T) {
	t.Parallel()
	encoded, err := newHasher(parameters{memoryKiB: 32, passes: 1, lanes: 1}).Hash(t.Context(), "secret")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	ok, needsRehash, err := newTestHasher().Verify(t.Context(), "wrong", encoded)
	if err != nil || ok || needsRehash {
		t.Fatalf("Verify = (%v, %v, %v), want (false, false, nil)", ok, needsRehash, err)
	}
}

func TestOperationsHonourCancellationWhileWaitingForASlot(t *testing.T) {
	t.Parallel()
	hasher := newTestHasher()
	for range maxConcurrentHashes {
		hasher.slots <- struct{}{}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	operations := map[string]func() error{
		"Hash": func() error { _, err := hasher.Hash(ctx, "secret"); return err },
		"Verify": func() error {
			_, _, err := hasher.Verify(ctx, "secret", "$argon2id$v=19$m=64,t=1,p=1$MDEyMzQ1Njc4OWFiY2RlZg$"+
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
			return err
		},
		"DummyVerify": func() error { return hasher.DummyVerify(ctx, "secret") },
	}
	for name, operation := range operations {
		if err := operation(); !errors.Is(err, context.Canceled) {
			t.Errorf("%s with cancelled context = %v, want context.Canceled", name, err)
		}
	}
}

func TestSlotsLimitConcurrentHashes(t *testing.T) {
	t.Parallel()
	hasher := newTestHasher()
	if capacity := cap(hasher.slots); capacity != maxConcurrentHashes || maxConcurrentHashes != 2 {
		t.Fatalf("slot capacity = %d, want 2", capacity)
	}
	for range maxConcurrentHashes {
		hasher.slots <- struct{}{}
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := hasher.Hash(ctx, "secret")
		done <- err
	}()
	<-hasher.slots // free one slot: the waiting Hash may now run
	if err := <-done; err != nil {
		t.Fatalf("Hash after a slot was freed: %v", err)
	}
	cancel()
	if len(hasher.slots) != maxConcurrentHashes-1 {
		t.Fatalf("Hash did not release its slot: %d in use", len(hasher.slots))
	}
}

func TestDummyVerifySucceedsWithoutRevealingAnything(t *testing.T) {
	t.Parallel()
	if err := newTestHasher().DummyVerify(t.Context(), "anything"); err != nil {
		t.Fatalf("DummyVerify: %v", err)
	}
}

func TestHashReportsRandomSourceFailure(t *testing.T) {
	t.Parallel()
	hasher := newTestHasher()
	hasher.random = failingReader{}
	if _, err := hasher.Hash(t.Context(), "secret"); err == nil {
		t.Fatal("Hash with a failing random source returned no error")
	}
	if len(hasher.slots) != 0 {
		t.Fatalf("failed Hash kept %d slots", len(hasher.slots))
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("no entropy") }

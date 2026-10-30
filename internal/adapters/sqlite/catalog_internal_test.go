package sqlite

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
)

// The cursor keeps every sort title exactly, including ones JSON would alter: a title that
// is not valid UTF-8 (a Linux file name) and one with a NUL byte after the separator.
func TestCursorRoundTripKeepsTheKeyExactly(t *testing.T) {
	t.Parallel()

	keys := []itemKey{
		{sortTitle: "big buck bunny", id: "8a9b0c1d-2e3f-4a5b-8c6d-7e8f9a0b1c2d"},
		{sortTitle: "", id: "id"},
		{sortTitle: "\xff\xfe broken utf-8", id: "id"},
		{sortTitle: "über \x00 nul", id: "id"},
		{sortTitle: "+/=?&#%", id: "id"},
	}

	for _, key := range keys {
		cursor := encodeCursor(key)

		got, err := decodeCursor(cursor)
		if err != nil || got != key {
			t.Errorf("decodeCursor(encodeCursor(%+v)) = %+v, %v, want the key", key, got, err)
		}

		for _, character := range cursor {
			if !isBase64URLCharacter(character) {
				t.Errorf("cursor %q contains %q, want base64url without padding", cursor, character)
			}
		}
	}
}

func isBase64URLCharacter(character rune) bool {
	return ('a' <= character && character <= 'z') || ('A' <= character && character <= 'Z') ||
		('0' <= character && character <= '9') || character == '-' || character == '_'
}

func TestDecodeCursorOfAnEmptyCursorIsTheZeroKey(t *testing.T) {
	t.Parallel()

	if key, err := decodeCursor(""); err != nil || key != (itemKey{}) {
		t.Errorf("decodeCursor(\"\") = %+v, %v, want the zero key", key, err)
	}
}

// Strict decoding refuses a cursor whose unused trailing bits are set, so one key has
// exactly one cursor.
func TestDecodeCursorRejectsNonCanonicalEncodings(t *testing.T) {
	t.Parallel()

	cursor := encodeCursor(itemKey{sortTitle: "t", id: "i"}) // 3 bytes → 4 characters, no spare bits
	shortCursor := encodeCursor(itemKey{sortTitle: "", id: "i"})

	// "i\x00" is 2 bytes: 3 characters with 2 unused bits in the last one.
	tampered := shortCursor[:2] + "B"
	if tampered == shortCursor {
		t.Fatalf("tampered cursor equals the original %q", shortCursor)
	}

	for _, input := range []string{tampered, cursor + "="} {
		if _, err := decodeCursor(input); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("decodeCursor(%q) error = %v, want domain.ErrInvalid", input, err)
		}
	}
}

// database/sql has already ended a transaction whose context was cancelled; ending it again
// reports sql.ErrTxDone, which is not a failure.
func TestEndingAnEndedTransactionIsNoError(t *testing.T) {
	t.Parallel()

	db, _ := openInternalTestDB(t)

	tests := []struct {
		name string
		pool *sql.DB
		end  func(*sql.Tx) error
	}{
		{name: "rollBack of a write transaction", pool: db.writer, end: rollBack},
		{name: "endReadTransaction", pool: db.reader, end: endReadTransaction},
	}

	for _, test := range tests {
		transaction, err := test.pool.BeginTx(t.Context(), nil)
		if err != nil {
			t.Fatalf("%s: BeginTx error = %v", test.name, err)
		}

		if err := transaction.Commit(); err != nil {
			t.Fatalf("%s: Commit error = %v", test.name, err)
		}

		if err := test.end(transaction); err != nil {
			t.Errorf("%s of a committed transaction error = %v, want nil", test.name, err)
		}
	}
}

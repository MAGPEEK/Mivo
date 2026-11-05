package sqlite

import (
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// A listing cursor is base64url (no padding) of fields separated by NUL:
//
//	"list", sort, direction, seed, null flag, value type, value, id, sort title
//
// It is bound to the sort, direction and seed that issued it, because a key of one order
// means nothing in another; the filter is not part of it (a changed filter just continues
// after the key). The value type is "n" (no value: title sort, or a NULL value), "i" (an
// integer) or "s" (text). The sort title comes last, so it may hold any byte but NUL (see
// cursorSeparator); a text value never holds NUL either (NFO text is XML, release dates are
// digits).
const (
	listingCursorTag    = "list"
	listingCursorFields = 9
	valueTypeNone       = "n"
	valueTypeInteger    = "i"
	valueTypeText       = "s"
	nullFlagSet         = "1"
	nullFlagClear       = "0"
)

// listingKey is the position of a row in a listing: its sort value (nil when the key has none
// or the value is NULL), sort title and ID.
type listingKey struct {
	value     any
	sortTitle string
	id        string
}

// listingCursorScope is what a listing cursor is bound to.
type listingCursorScope struct {
	sort       library.ItemSort
	descending bool
	seed       int64
}

func (scope listingCursorScope) direction() string {
	if scope.descending {
		return descendingDirection
	}

	return ascendingDirection
}

func encodeListingCursor(scope listingCursorScope, key listingKey) string {
	nullFlag, valueType, value := nullFlagSet, valueTypeNone, ""

	switch typed := key.value.(type) {
	case int64:
		nullFlag, valueType, value = nullFlagClear, valueTypeInteger, strconv.FormatInt(typed, numberBase)
	case string:
		nullFlag, valueType, value = nullFlagClear, valueTypeText, typed
	}

	fields := []string{
		listingCursorTag, string(scope.sort), scope.direction(), strconv.FormatInt(scope.seed, numberBase),
		nullFlag, valueType, value, key.id, key.sortTitle,
	}

	return base64.RawURLEncoding.EncodeToString([]byte(strings.Join(fields, cursorSeparator)))
}

// decodeListingCursor reads a cursor of the listing. An empty cursor reports no key; one not
// issued for the sort and direction wraps domain.ErrInvalid. The seed comes back from the
// cursor: the caller checks it against the request's.
func decodeListingCursor(cursor string, scope listingCursorScope) (key listingKey, seed int64, found bool, err error) {
	if cursor == "" {
		return listingKey{}, 0, false, nil
	}

	raw, err := decodeCursorBytes(cursor)
	if err != nil {
		return listingKey{}, 0, false, err
	}

	fields := strings.SplitN(string(raw), cursorSeparator, listingCursorFields)
	if len(fields) != listingCursorFields || fields[0] != listingCursorTag || fields[1] != string(scope.sort) ||
		fields[2] != scope.direction() || fields[7] == "" {
		return listingKey{}, 0, false, fmt.Errorf("cursor was not issued for this listing: %w", domain.ErrInvalid)
	}

	seed, seedErr := strconv.ParseInt(fields[3], numberBase, numberBitSize)
	value, valueErr := cursorValue(fields[4], fields[5], fields[6])

	// The API carries a seed as a non-negative int32; anything else was not issued here.
	if seedErr != nil || valueErr != nil || seed < 0 || seed > math.MaxInt32 {
		return listingKey{}, 0, false, fmt.Errorf("cursor has no listing key: %w", domain.ErrInvalid)
	}

	return listingKey{value: value, id: fields[7], sortTitle: fields[8]}, seed, true, nil
}

func cursorValue(nullFlag, valueType, text string) (any, error) {
	switch {
	case nullFlag == nullFlagSet && valueType == valueTypeNone && text == "":
		return nil, nil
	case nullFlag == nullFlagClear && valueType == valueTypeInteger:
		number, err := strconv.ParseInt(text, numberBase, numberBitSize)
		if err != nil {
			return nil, fmt.Errorf("cursor value: %w", err)
		}

		return number, nil
	case nullFlag == nullFlagClear && valueType == valueTypeText:
		return text, nil
	default:
		return nil, fmt.Errorf("cursor value type %q: %w", valueType, domain.ErrInvalid)
	}
}

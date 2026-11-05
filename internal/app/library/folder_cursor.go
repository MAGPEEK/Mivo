package library

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// A folder view cursor is base64url (no padding) of "folders NUL <path> NUL <offset>": the
// position of the next entry, bound to the listed path so it cannot page another folder. A
// valid folder path never contains NUL (library.ParseFolderPath).
const (
	folderCursorTag       = "folders"
	folderCursorSeparator = "\x00"
	folderCursorFields    = 3
)

func encodeFolderCursor(path library.FolderPath, offset int) string {
	text := strings.Join([]string{folderCursorTag, path.String(), strconv.Itoa(offset)}, folderCursorSeparator)

	return base64.RawURLEncoding.EncodeToString([]byte(text))
}

// decodeFolderCursor returns 0 for an empty cursor. A cursor not issued for this path wraps
// domain.ErrInvalid.
func decodeFolderCursor(cursor string, path library.FolderPath) (int, error) {
	if cursor == "" {
		return 0, nil
	}

	raw, err := base64.RawURLEncoding.Strict().DecodeString(cursor)
	if err != nil || strings.ContainsAny(cursor, "\r\n") {
		return 0, fmt.Errorf("folder cursor is not base64url: %w", domain.ErrInvalid)
	}

	fields := strings.Split(string(raw), folderCursorSeparator)
	if len(fields) != folderCursorFields || fields[0] != folderCursorTag || fields[1] != path.String() {
		return 0, fmt.Errorf("cursor was not issued for this folder: %w", domain.ErrInvalid)
	}

	offset, err := strconv.Atoi(fields[2])
	if err != nil || offset <= 0 || strconv.Itoa(offset) != fields[2] {
		return 0, fmt.Errorf("folder cursor has no position: %w", domain.ErrInvalid)
	}

	return offset, nil
}

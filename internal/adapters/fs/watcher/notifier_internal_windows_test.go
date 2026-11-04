package watcher

import (
	"encoding/binary"
	"path/filepath"
	"slices"
	"testing"
	"unicode/utf16"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/sys/windows"
)

// notifyRecord encodes one FILE_NOTIFY_INFORMATION record; next is the offset of the
// following record or 0 for the last one.
func notifyRecord(next, action uint32, name string) []byte {
	units := utf16.Encode([]rune(name))
	record := make([]byte, recordHeaderSize+len(units)*utf16UnitSize)

	binary.LittleEndian.PutUint32(record[recordNextOffset:], next)
	binary.LittleEndian.PutUint32(record[recordActionOffset:], action)
	binary.LittleEndian.PutUint32(record[recordNameLengthOffset:], uint32(len(units)*utf16UnitSize)) //nolint:gosec // test names are a few bytes long

	for index, unit := range units {
		binary.LittleEndian.PutUint16(record[recordHeaderSize+index*utf16UnitSize:], unit)
	}

	return record
}

func TestParseRecords(t *testing.T) {
	t.Parallel()

	root := `C:\Media`
	first := notifyRecord(0, windows.FILE_ACTION_RENAMED_OLD_NAME, `Old\Film.mkv`)
	second := notifyRecord(0, windows.FILE_ACTION_RENAMED_NEW_NAME, `Neu\Fïlm 🎬.mkv`)
	third := notifyRecord(0, windows.FILE_ACTION_REMOVED, "gone.mkv")
	unknown := notifyRecord(0, 0, "ignored")
	modified := notifyRecord(0, windows.FILE_ACTION_MODIFIED, "grows.mkv")

	// Offsets chain the records; the last one ends with 0.
	chain := [][]byte{first, second, unknown, third, modified}
	buffer := []byte{}

	for index, record := range chain {
		if index < len(chain)-1 {
			binary.LittleEndian.PutUint32(record[recordNextOffset:], uint32(len(record))) //nolint:gosec // test records are a few bytes long
		}

		buffer = append(buffer, record...)
	}

	want := []fsnotify.Event{
		{Name: filepath.Join(root, `Old\Film.mkv`), Op: fsnotify.Rename},
		{Name: filepath.Join(root, `Neu\Fïlm 🎬.mkv`), Op: fsnotify.Create},
		{Name: filepath.Join(root, "gone.mkv"), Op: fsnotify.Remove},
		{Name: filepath.Join(root, "grows.mkv"), Op: fsnotify.Write},
	}

	if got := parseRecords(root, buffer); !slices.Equal(got, want) {
		t.Errorf("parseRecords = %v, want %v", got, want)
	}
}

func TestParseRecordsStopsAtATruncatedRecord(t *testing.T) {
	t.Parallel()

	record := notifyRecord(0, windows.FILE_ACTION_ADDED, "film.mkv")

	if got := parseRecords(`C:\Media`, record[:len(record)-1]); len(got) != 0 {
		t.Errorf("parseRecords(truncated) = %v, want none", got)
	}

	if got := parseRecords(`C:\Media`, record[:recordHeaderSize-1]); len(got) != 0 {
		t.Errorf("parseRecords(short header) = %v, want none", got)
	}
}

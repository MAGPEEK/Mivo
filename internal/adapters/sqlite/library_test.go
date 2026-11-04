package sqlite_test

import (
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"
)

// Roots are opaque to the repository; these look like what the configuration produces.
const (
	moviesRoot = "/media/movies"
	showsRoot  = "/media/shows"
	animeRoot  = "/media/anime"
)

var errIDsExhausted = errors.New("test id generator exhausted")

// syncStartMilli is the first instant of the test clock (2026-10-30 12:00 UTC) in Unix
// milliseconds.
const syncStartMilli int64 = 1_793_361_600_000

// testClock is a manually advanced clock for created_at/updated_at assertions.
type testClock struct {
	mutex sync.Mutex
	now   time.Time
}

func newTestClock() *testClock {
	return &testClock{now: time.UnixMilli(syncStartMilli).UTC()}
}

func (clock *testClock) Now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()

	return clock.now
}

func (clock *testClock) Advance(duration time.Duration) {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()

	clock.now = clock.now.Add(duration)
}

// idSequence returns a newID function that hands out ids in order and fails when they are
// used up, plus a function reporting how many it handed out.
func idSequence(ids ...string) (func() (string, error), func() int) {
	var (
		mutex sync.Mutex
		next  int
	)

	newID := func() (string, error) {
		mutex.Lock()
		defer mutex.Unlock()

		if next == len(ids) {
			return "", errIDsExhausted
		}

		next++

		return ids[next-1], nil
	}

	used := func() int {
		mutex.Lock()
		defer mutex.Unlock()

		return next
	}

	return newID, used
}

func rowCount(t *testing.T, inspection *sql.DB, table string) int {
	t.Helper()

	var count int

	// The table name comes from the test itself, never from input.
	err := inspection.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&count)
	if err != nil {
		t.Fatalf("count rows of %s: %v", table, err)
	}

	return count
}

// insertMediaFileQuery stores a successfully probed file; parameters: id, item_id,
// library_id, relative_path.
const insertMediaFileQuery = `INSERT INTO media_files (id, item_id, library_id, root_id, relative_path,
	size_bytes, modified_at, container, format_name, duration_ms, bit_rate, probed_at, probe_error)
	VALUES (?, ?, ?, (SELECT id FROM library_roots WHERE library_id = ?3 AND position = 0), ?, 1048576, 0, 'mkv', 'matroska,webm', 30000, 2000000, 0, NULL)`

// insertMediaStreamQuery stores a stream; parameters: media_file_id, stream_index, kind.
const insertMediaStreamQuery = `INSERT INTO media_streams (media_file_id, stream_index, kind, codec,
	codec_tag, profile, level, pixel_format, bit_depth, width, height, frame_rate, color_primaries,
	color_transfer, color_space, hdr, channels, channel_layout, sample_rate, language, title,
	is_default, is_forced, is_attached_picture, bit_rate)
	VALUES (?, ?, ?, 'h264', NULL, 'High', 40, 'yuv420p', 8, 1920, 1080, '24000/1001', '', '', '',
	'', 0, '', 0, 'eng', '', 1, 0, 0, 0)`

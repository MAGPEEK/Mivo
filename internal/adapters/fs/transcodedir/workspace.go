// Package transcodedir manages the per-session working directories below
// <transcode>/sessions/ in which FFmpeg writes HLS playlists and segments (ADR 0024).
//
// Safety rule: the package only ever creates or deletes entries of <transcode>/sessions whose
// names are canonical lowercase UUIDs, so a misconfigured transcode path cannot destroy user
// files. Every access below the transcode root goes through os.Root, which refuses names and
// symlinks that leave the directory, and the sessions directory must be a plain directory, not
// a symlink or junction.
package transcodedir

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/magpeek/mivo/internal/domain"
)

const (
	// sessionsDirName is the only directory below the transcode root that this package manages.
	sessionsDirName = "sessions"
	// directoryMode keeps session output private to the server user and its group.
	directoryMode fs.FileMode = 0o750
)

// ErrSessionsNotPlainDirectory reports that <transcode>/sessions is a symlink, junction or
// other non-directory entry. Such an entry is refused instead of followed: a link to "." or to
// a user folder would otherwise let Purge delete UUID-named entries there.
var ErrSessionsNotPlainDirectory = errors.New("sessions entry is not a plain directory")

// Workspace creates, opens and removes session directories below <root>/sessions. It holds
// no open handles, so it needs no Close and is safe for concurrent use.
type Workspace struct {
	root        string
	sessionsDir string
	retry       retryPolicy
}

// New returns a Workspace for the transcode root. The root must be an absolute path; it and
// its sessions directory are created on first use, not here.
func New(root string) (*Workspace, error) {
	return newWorkspace(root, defaultRetryPolicy())
}

func newWorkspace(root string, retry retryPolicy) (*Workspace, error) {
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("transcode workspace root %q: must be an absolute path: %w",
			root, domain.ErrInvalid)
	}

	cleanRoot := filepath.Clean(root)

	return &Workspace{
		root:        cleanRoot,
		sessionsDir: filepath.Join(cleanRoot, sessionsDirName),
		retry:       retry,
	}, nil
}

// Create makes the directory of a new session and returns its path. The session ID must be a
// canonical lowercase UUID (else domain.ErrInvalid); an existing directory of the same session
// is reported as domain.ErrConflict and left untouched.
func (w *Workspace) Create(ctx context.Context, sessionID string) (string, error) {
	if err := requireSessionID(sessionID); err != nil {
		return "", fmt.Errorf("create session directory: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("create session directory %s: %w", sessionID, err)
	}

	sessions, err := w.openSessions(true)
	if err != nil {
		return "", fmt.Errorf("create session directory %s: %w", sessionID, err)
	}

	defer closeRoot(sessions)

	err = sessions.Mkdir(sessionID, directoryMode)
	if errors.Is(err, fs.ErrExist) {
		return "", fmt.Errorf("create session directory %s: already exists: %w", sessionID, domain.ErrConflict)
	}

	if err != nil {
		return "", fmt.Errorf("create session directory %s: %w", sessionID, err)
	}

	return filepath.Join(w.sessionsDir, sessionID), nil
}

// Remove deletes the directory of a session recursively. Removing a session without a
// directory is not an error. Deletion is retried while files are still locked (Windows keeps
// the files of a just-killed FFmpeg locked for a moment) until the retries run out or ctx ends.
func (w *Workspace) Remove(ctx context.Context, sessionID string) error {
	if err := requireSessionID(sessionID); err != nil {
		return fmt.Errorf("remove session directory: %w", err)
	}

	sessions, err := w.openSessions(false)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("remove session directory %s: %w", sessionID, err)
	}

	defer closeRoot(sessions)

	err = removeWithRetry(ctx, w.retry, func() error { return sessions.RemoveAll(sessionID) })
	if err != nil {
		return fmt.Errorf("remove session directory %s: %w", sessionID, err)
	}

	return nil
}

// OpenFile opens a file inside the directory of a session for reading. The name is not
// validated here (the HTTP adapter allows only known file names), but os.Root rejects names
// and symlinks that leave the session directory. A missing session or file is reported as
// domain.ErrNotFound.
func (w *Workspace) OpenFile(sessionID, name string) (*os.File, error) {
	if err := requireSessionID(sessionID); err != nil {
		return nil, fmt.Errorf("open session file: %w", err)
	}

	file, err := w.openSessionFile(sessionID, name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("open session file %s/%s: %w: %w", sessionID, name, domain.ErrNotFound, err)
	}

	if err != nil {
		return nil, fmt.Errorf("open session file %s/%s: %w", sessionID, name, err)
	}

	return file, nil
}

func (w *Workspace) openSessionFile(sessionID, name string) (*os.File, error) {
	sessions, err := w.openSessions(false)
	if err != nil {
		return nil, err
	}

	defer closeRoot(sessions)

	// A root per session keeps one session from reading another's files through "..".
	session, err := sessions.OpenRoot(sessionID)
	if err != nil {
		return nil, fmt.Errorf("open session root: %w", err)
	}

	defer closeRoot(session)

	file, err := session.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}

	return file, nil
}

// openSessions opens <root>/sessions as an os.Root, creating it (and the transcode root) when
// create is set. The entry must be a plain directory; opening it through the transcode root's
// os.Root additionally confines it to the transcode root.
func (w *Workspace) openSessions(create bool) (*os.Root, error) {
	if create {
		if err := os.MkdirAll(w.root, directoryMode); err != nil {
			return nil, fmt.Errorf("create transcode root %s: %w", w.root, err)
		}
	}

	root, err := os.OpenRoot(w.root)
	if err != nil {
		return nil, fmt.Errorf("open transcode root %s: %w", w.root, err)
	}

	defer closeRoot(root)

	if create {
		if err := ensureSessionsDir(root); err != nil {
			return nil, fmt.Errorf("create %s: %w", w.sessionsDir, err)
		}
	}

	if err := requirePlainDirectory(root, sessionsDirName); err != nil {
		return nil, fmt.Errorf("check %s: %w", w.sessionsDir, err)
	}

	sessions, err := root.OpenRoot(sessionsDirName)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", w.sessionsDir, err)
	}

	return sessions, nil
}

// ensureSessionsDir creates the sessions directory. An existing entry is left for
// requirePlainDirectory to judge, so a link or file is reported clearly instead of as
// "file exists".
func ensureSessionsDir(root *os.Root) error {
	err := root.Mkdir(sessionsDirName, directoryMode)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("make directory: %w", err)
	}

	return nil
}

// requirePlainDirectory refuses symlinks, junctions (reported as irregular on Windows) and
// every other non-directory entry. Lstat does not follow the entry, so a link is seen as such.
func requirePlainDirectory(root *os.Root, name string) error {
	info, err := root.Lstat(name)
	if err != nil {
		return fmt.Errorf("inspect entry: %w", err)
	}

	if entryType := info.Mode().Type(); entryType != fs.ModeDir {
		return fmt.Errorf("entry type %q: %w", entryType.String(), ErrSessionsNotPlainDirectory)
	}

	return nil
}

// closeRoot closes a root that was only read from or used for directory operations; a close
// error there cannot lose data, so it is deliberately ignored.
func closeRoot(root *os.Root) {
	_ = root.Close()
}

func requireSessionID(sessionID string) error {
	if !isSessionID(sessionID) {
		return fmt.Errorf("session id %q: not a canonical lowercase UUID: %w", sessionID, domain.ErrInvalid)
	}

	return nil
}

package folderbrowse

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

const (
	// DefaultTimeout bounds every call; a hung network mount must not block the API.
	DefaultTimeout = 5 * time.Second
	// DefaultListingCap is the maximum number of entries List returns.
	DefaultListingCap = 1000
	// rootIDHexDigits is the length of an opaque root id.
	rootIDHexDigits = 16
	// readabilityConcurrency bounds parallel open checks on a slow share.
	readabilityConcurrency = 4
	// readBatch is the number of directory entries read per call, so cancellation is seen.
	readBatch = 256
)

// Options configures a Browser. Zero values select the defaults.
type Options struct {
	// Reserved are absolute directories (config, data, cache, transcode) that are never
	// listed or accepted, nor anything inside them.
	Reserved []string
	// Timeout bounds one call; DefaultTimeout when zero.
	Timeout time.Duration
	// ListingCap limits List entries; DefaultListingCap when zero.
	ListingCap int
}

// Root is a configured browse root.
type Root struct {
	ID        string
	Path      string
	Available bool
}

// Folder is one listed sub-directory; Path is relative to the root with "/" separators.
type Folder struct {
	Name string
	Path string
}

// Listing is the result of List.
type Listing struct {
	Folders []Folder
	// UnreadableCount counts the unreadable folders examined; it is partial when Truncated,
	// because the checks stop one readable folder past the cap.
	UnreadableCount int
	Truncated       bool
}

type rootEntry struct {
	id   string
	path string
}

// Browser is the file system port of the folder browser.
type Browser struct {
	roots    []rootEntry
	reserved []string
	timeout  time.Duration
	capacity int

	// statRoot is the availability probe; tests replace it to simulate a hung mount.
	statRoot func(path string) bool

	mu       sync.Mutex
	inflight map[string]*availabilityCheck
}

// New creates a Browser for the given absolute root paths.
func New(roots []string, options Options) *Browser {
	browser := &Browser{
		timeout:  options.Timeout,
		capacity: options.ListingCap,
		inflight: map[string]*availabilityCheck{},
		statRoot: statDirectory,
	}
	if browser.timeout <= 0 {
		browser.timeout = DefaultTimeout
	}

	if browser.capacity <= 0 {
		browser.capacity = DefaultListingCap
	}

	for _, root := range roots {
		cleaned := filepath.Clean(root)
		browser.roots = append(browser.roots, rootEntry{id: RootID(cleaned), path: cleaned})
	}

	for _, reserved := range options.Reserved {
		browser.reserved = append(browser.reserved, filepath.Clean(reserved))
	}

	return browser
}

// RootID returns the opaque, restart-stable id of a root path.
func RootID(path string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(path)))

	return hex.EncodeToString(sum[:])[:rootIDHexDigits]
}

func (b *Browser) rootByID(rootID string) (rootEntry, error) {
	for _, root := range b.roots {
		if root.id == rootID {
			return root, nil
		}
	}

	return rootEntry{}, fmt.Errorf("browse root %q: %w", rootID, domain.ErrInvalid)
}

// bounded runs work under the call timeout. Blocking file system calls ignore ctx, so the
// work runs in its own goroutine and is abandoned when the time is up; work checks ctx
// between steps and ends soon after.
func bounded[T any](ctx context.Context, timeout time.Duration, work func(context.Context) (T, error)) (T, error) {
	if err := ctx.Err(); err != nil {
		var zero T

		return zero, fmt.Errorf("browse folders: %w", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type outcome struct {
		value T
		err   error
	}

	done := make(chan outcome, 1)

	go func() {
		value, err := work(callCtx)
		done <- outcome{value, err}
	}()

	select {
	case result := <-done:
		return result.value, result.err
	case <-callCtx.Done():
		var zero T
		if ctx.Err() != nil {
			return zero, fmt.Errorf("browse folders: %w", ctx.Err())
		}

		return zero, fmt.Errorf("browse folders timed out after %s: %w", timeout, domain.ErrUnavailable)
	}
}

// gate lets a call proceed only while the root answers. It shares the one in-flight stat per
// root, so a hung mount blocks at most that stat and not one goroutine per call.
func (b *Browser) gate(ctx context.Context, root rootEntry) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("browse folders: %w", err)
	}

	if b.available(ctx, root.path) {
		return nil
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("browse folders: %w", err)
	}

	return fmt.Errorf("browse root %s: %w", root.id, domain.ErrUnavailable)
}

// openRoot opens a root; a missing or unreadable root is unavailable (NAS shares mount late).
// The error names no path: clients see the root id only.
func openRoot(path string) (*os.Root, error) {
	opened, err := os.OpenRoot(path)
	if err != nil {
		return nil, fmt.Errorf("open browse root: %w: %w", domain.ErrUnavailable, withoutPath(err))
	}

	return opened, nil
}

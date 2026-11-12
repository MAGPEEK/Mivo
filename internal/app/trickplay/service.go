package trickplay

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	appaccess "github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/domain"
)

const (
	// MaxPendingJobs caps the queued generations; a full queue drops further requests.
	MaxPendingJobs = 64
	// DefaultMaxCacheBytes caps <cache>/trickplay when Options.MaxCacheBytes is zero.
	DefaultMaxCacheBytes int64 = 2 << 30
	// IdlePollInterval is how often a job re-checks for running re-encodes.
	IdlePollInterval = 30 * time.Second
	// IdleGiveUpAfter is how long a job waits for an idle server before it is dropped.
	IdleGiveUpAfter = time.Hour
	// sheetNameFormat is the generator's file name, numbered from 1.
	sheetNameFormat = "sheet-%03d.jpg"
)

// Status is the polling state of one file's trickplay.
type Status string

const (
	// StatusAbsent means nothing was requested (or the request was dropped or expired).
	StatusAbsent Status = "absent"
	// StatusPending means the job is queued or running.
	StatusPending Status = "pending"
	// StatusReady means the manifest exists for the current file.
	StatusReady Status = "ready"
	// StatusFailed means generation failed in this process and is not retried.
	StatusFailed Status = "failed"
)

// Options configures New. Files, Paths, Store, Cache, Sessions, Clock and Logger are required;
// a nil Generator (direct-play-only server, FFmpeg missing) makes Request answer
// domain.ErrUnavailable.
type Options struct {
	Files     FileSource
	Paths     PathResolver
	Generator Generator
	Store     Store
	Cache     Cache
	Sessions  SessionLister
	Clock     Clock
	Logger    *slog.Logger
	// MaxSessions is the configured max_sessions; above 1 a job never waits for idleness.
	MaxSessions int
	// MaxCacheBytes caps the cache; zero means DefaultMaxCacheBytes.
	MaxCacheBytes int64
}

// Service generates and looks up trickplay.
type Service struct {
	opts Options

	mu      sync.Mutex
	records map[string]record
	queue   []string
	closed  bool
	wake    chan struct{}
	cancel  context.CancelFunc
	done    chan struct{}
}

// record is what the process knows about one file's job.
type record struct {
	status Status
	key    string
}

// New validates options. It starts nothing; call Start.
func New(options Options) (*Service, error) {
	switch {
	case options.Files == nil, options.Paths == nil, options.Store == nil, options.Cache == nil,
		options.Sessions == nil, options.Clock == nil, options.Logger == nil:
		return nil, fmt.Errorf("new trickplay service: a required option is nil: %w", domain.ErrInvalid)
	}

	if options.MaxCacheBytes <= 0 {
		options.MaxCacheBytes = DefaultMaxCacheBytes
	}

	return &Service{
		opts: options, records: map[string]record{}, wake: make(chan struct{}, 1),
		done: make(chan struct{}),
	}, nil
}

// Request queues the generation for a file and returns at once. It is idempotent: a file
// that is queued, running, done or failed gets no second job, and a full queue drops the
// request without an error. Errors wrap domain.ErrNotFound (unknown or hidden file),
// domain.ErrInvalid (no video, not probed, disc), domain.ErrUnavailable (no generator or
// shutting down).
func (s *Service) Request(ctx context.Context, fileID string) error {
	file, err := s.visibleFile(ctx, fileID)
	if err != nil {
		return err
	}

	if err := checkGeneratable(file); err != nil {
		return err
	}

	if s.opts.Generator == nil {
		return fmt.Errorf("trickplay %s: no generator: %w", file.ID, domain.ErrUnavailable)
	}

	key := keyOf(file)

	stored, err := s.opts.Store.Load(ctx, file.ID)

	switch {
	case err == nil && stored.Key == key:
		s.remember(file.ID, record{status: StatusReady, key: key})
		return nil
	case err != nil && !errors.Is(err, domain.ErrNotFound):
		return fmt.Errorf("trickplay %s: %w", file.ID, err)
	}

	return s.enqueue(file.ID, key)
}

// Status reports the polling state of a visible file.
func (s *Service) Status(ctx context.Context, fileID string) (Status, error) {
	file, err := s.visibleFile(ctx, fileID)
	if err != nil {
		return StatusAbsent, err
	}

	if _, err := s.readyManifest(ctx, file); err == nil {
		return StatusReady, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return StatusAbsent, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if rec, ok := s.records[file.ID]; ok && rec.key == keyOf(file) && rec.status != StatusReady {
		return rec.status, nil
	}

	return StatusAbsent, nil
}

// Manifest returns the manifest of a visible file's trickplay, or an error wrapping
// domain.ErrNotFound when it does not exist (yet), failed or belongs to an older version of
// the file.
func (s *Service) Manifest(ctx context.Context, fileID string) (Manifest, error) {
	file, err := s.visibleFile(ctx, fileID)
	if err != nil {
		return Manifest{}, err
	}

	return s.readyManifest(ctx, file)
}

// Sheet returns the absolute path of a sprite sheet. sheet is 0-based like the API's
// sheetUrls; the files on disk are numbered from 1.
func (s *Service) Sheet(ctx context.Context, fileID string, sheet int) (string, error) {
	file, err := s.visibleFile(ctx, fileID)
	if err != nil {
		return "", err
	}

	manifest, err := s.readyManifest(ctx, file)
	if err != nil {
		return "", err
	}

	if sheet < 0 || sheet >= manifest.SheetCount {
		return "", fmt.Errorf("trickplay %s: sheet %d: %w", file.ID, sheet, domain.ErrNotFound)
	}

	return filepath.Join(s.opts.Store.Directory(file.ID), fmt.Sprintf(sheetNameFormat, sheet+1)), nil
}

// Start prunes the cache of vanished files and staging leftovers, then runs the worker until
// Close or ctx ends.
func (s *Service) Start(ctx context.Context) error {
	if err := s.opts.Cache.SweepStaging(ctx); err != nil {
		return fmt.Errorf("start trickplay: sweep staging: %w", err)
	}

	if err := s.pruneVanished(ctx); err != nil {
		return fmt.Errorf("start trickplay: %w", err)
	}

	workerCtx, cancel := context.WithCancel(ctx)

	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()

	go func() {
		defer close(s.done)
		s.work(workerCtx)
	}()

	return nil
}

// Close stops the worker, cancelling a running generation, and refuses further requests.
func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	cancel := s.cancel
	s.mu.Unlock()

	if cancel == nil {
		return
	}

	cancel()
	<-s.done
}

func (s *Service) visibleFile(ctx context.Context, fileID string) (fileView, error) {
	restriction, err := appaccess.RestrictionFrom(ctx)
	if err != nil {
		return fileView{}, err
	}

	playable, err := s.opts.Files.PlayableFile(ctx, fileID, restriction)
	if err != nil {
		return fileView{}, fmt.Errorf("trickplay %s: %w", fileID, err)
	}

	return newFileView(playable), nil
}

// readyManifest loads the manifest and rejects one of an older file version.
func (s *Service) readyManifest(ctx context.Context, file fileView) (Manifest, error) {
	stored, err := s.opts.Store.Load(ctx, file.ID)
	if err != nil {
		return Manifest{}, fmt.Errorf("trickplay %s: %w", file.ID, err)
	}

	if stored.Key != keyOf(file) {
		return Manifest{}, fmt.Errorf("trickplay %s: stale: %w", file.ID, domain.ErrNotFound)
	}

	if err := s.opts.Store.Touch(ctx, file.ID); err != nil {
		s.opts.Logger.Debug("touch trickplay entry", "file", file.ID, "error", err)
	}

	return stored.Manifest, nil
}

func (s *Service) remember(fileID string, rec record) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.records[fileID] = rec
}

func (s *Service) enqueue(fileID, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("trickplay %s: shutting down: %w", fileID, domain.ErrUnavailable)
	}

	// A ready record does not block: the store was just found without the manifest, so the
	// entry was evicted or removed.
	if rec, ok := s.records[fileID]; ok && rec.key == key && rec.status != StatusReady {
		return nil
	}

	if len(s.queue) >= MaxPendingJobs {
		s.opts.Logger.Debug("trickplay queue full, request dropped", "file", fileID)
		return nil
	}

	s.records[fileID] = record{status: StatusPending, key: key}
	s.queue = append(s.queue, fileID)

	select {
	case s.wake <- struct{}{}:
	default:
	}

	return nil
}

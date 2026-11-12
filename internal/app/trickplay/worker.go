package trickplay

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// errIdleTimeout ends a job that waited IdleGiveUpAfter for a server without re-encodes.
var errIdleTimeout = errors.New("server stayed busy")

// fileView is the part of a playable file the service uses.
type fileView struct {
	ID          string
	LibraryRoot string
	Relative    string
	Size        int64
	ModifiedAt  int64
	Disc        bool
	HasVideo    bool
	Probed      bool
}

func newFileView(playable appplayback.PlayableFile) fileView {
	file := playable.File
	_, hasVideo := file.Description.PrimaryVideo()

	return fileView{
		ID: file.ID, LibraryRoot: playable.LibraryRoot, Relative: file.RelativePath,
		Size: file.Size, ModifiedAt: file.ModifiedAt.UnixNano(),
		Disc: file.Layout.IsDisc(), HasVideo: hasVideo, Probed: file.Probed(),
	}
}

// keyOf identifies the version of the source file; a changed file regenerates.
func keyOf(file fileView) string {
	return strconv.FormatInt(file.Size, 10) + "-" + strconv.FormatInt(file.ModifiedAt, 10)
}

func checkGeneratable(file fileView) error {
	switch {
	case file.Disc:
		return fmt.Errorf("trickplay %s: disc image: %w", file.ID, domain.ErrInvalid)
	case !file.Probed:
		return fmt.Errorf("trickplay %s: file not probed: %w", file.ID, domain.ErrInvalid)
	case !file.HasVideo:
		return fmt.Errorf("trickplay %s: no video stream: %w", file.ID, domain.ErrInvalid)
	}

	return nil
}

// work runs queued jobs one at a time until ctx ends.
func (s *Service) work(ctx context.Context) {
	for {
		fileID, ok := s.nextJob(ctx)
		if !ok {
			return
		}

		s.runJob(ctx, fileID)
	}
}

func (s *Service) nextJob(ctx context.Context) (string, bool) {
	for {
		s.mu.Lock()
		if len(s.queue) > 0 {
			fileID := s.queue[0]
			s.queue = s.queue[1:]
			s.mu.Unlock()

			return fileID, true
		}
		s.mu.Unlock()

		select {
		case <-ctx.Done():
			return "", false
		case <-s.wake:
		}
	}
}

func (s *Service) runJob(ctx context.Context, fileID string) {
	if err := s.waitForIdle(ctx); err != nil {
		s.forget(fileID)

		if !errors.Is(err, context.Canceled) {
			s.opts.Logger.Warn("trickplay job dropped", "file", fileID, "error", err)
		}

		return
	}

	err := s.generate(ctx, fileID)

	switch {
	case err == nil:
	case ctx.Err() != nil, errors.Is(err, domain.ErrNotFound):
		s.forget(fileID)
	default:
		s.fail(fileID, err)
	}
}

func (s *Service) forget(fileID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.records, fileID)
}

func (s *Service) fail(fileID string, cause error) {
	s.mu.Lock()
	rec := s.records[fileID]
	rec.status = StatusFailed
	s.records[fileID] = rec
	s.mu.Unlock()

	s.opts.Logger.Warn("trickplay generation failed", "file", fileID, "error", cause)
}

// generate renders one file. The worker acts for the system, so the file is read without a
// viewer's restriction; the viewer was checked in Request.
func (s *Service) generate(ctx context.Context, fileID string) error {
	playable, err := s.opts.Files.PlayableFile(ctx, fileID, access.Unrestricted())
	if err != nil {
		return fmt.Errorf("load file: %w", err)
	}

	file := newFileView(playable)

	source, err := s.opts.Paths.Resolve(file.LibraryRoot, file.Relative)
	if err != nil {
		return fmt.Errorf("resolve source: %w", err)
	}

	manifest, err := s.opts.Generator.GenerateTrickplay(ctx, source, s.opts.Store.Directory(file.ID))
	if err != nil {
		return fmt.Errorf("generate: %w", err)
	}

	key := keyOf(file)
	if err := s.opts.Store.Save(ctx, file.ID, Stored{Manifest: manifest, Key: key}); err != nil {
		return fmt.Errorf("save manifest: %w", err)
	}

	s.remember(file.ID, record{status: StatusReady, key: key})
	s.evict(ctx, file.ID)

	return nil
}

// waitForIdle blocks until no viewer re-encode runs, polling every IdlePollInterval and
// giving up after IdleGiveUpAfter.
func (s *Service) waitForIdle(ctx context.Context) error {
	deadline := s.opts.Clock.Now().Add(IdleGiveUpAfter)

	for {
		if !s.busy(ctx) {
			return nil
		}

		if !s.opts.Clock.Now().Before(deadline) {
			return errIdleTimeout
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for idle server: %w", ctx.Err())
		case <-s.opts.Clock.After(IdlePollInterval):
		}
	}
}

// busy reports a running re-encode. A server that allows several sessions has capacity to
// spare; an unreadable session list counts as busy (fail safe for the viewer).
func (s *Service) busy(ctx context.Context) bool {
	if s.opts.MaxSessions > 1 {
		return false
	}

	sessions, err := s.opts.Sessions.List(ctx)
	if err != nil {
		s.opts.Logger.Debug("list sessions for trickplay", "error", err)
		return true
	}

	for _, session := range sessions {
		if session.Mode == playback.ModeTranscode {
			return true
		}
	}

	return false
}

// evict removes least recently used entries until the cache fits, never the one just written.
func (s *Service) evict(ctx context.Context, keep string) {
	entries, err := s.opts.Cache.Entries(ctx)
	if err != nil {
		s.opts.Logger.Warn("list trickplay cache", "error", err)
		return
	}

	var total int64
	for _, entry := range entries {
		total += entry.Bytes
	}

	slices.SortFunc(entries, func(a, b Entry) int { return a.LastUsed.Compare(b.LastUsed) })

	for _, entry := range entries {
		if total <= s.opts.MaxCacheBytes {
			return
		}

		if entry.FileID == keep {
			continue
		}

		if err := s.opts.Cache.Remove(ctx, entry.FileID); err != nil {
			s.opts.Logger.Warn("evict trickplay entry", "file", entry.FileID, "error", err)
			continue
		}

		total -= entry.Bytes
	}
}

// pruneVanished removes cache entries whose file left the catalog.
func (s *Service) pruneVanished(ctx context.Context) error {
	entries, err := s.opts.Cache.Entries(ctx)
	if err != nil {
		return fmt.Errorf("list cache: %w", err)
	}

	for _, entry := range entries {
		_, err := s.opts.Files.PlayableFile(ctx, entry.FileID, access.Unrestricted())

		switch {
		case err == nil:
		case errors.Is(err, domain.ErrNotFound):
			if err := s.opts.Cache.Remove(ctx, entry.FileID); err != nil {
				return fmt.Errorf("remove %s: %w", entry.FileID, err)
			}
		default:
			s.opts.Logger.Warn("check trickplay entry", "file", entry.FileID, "error", err)
		}
	}

	return nil
}

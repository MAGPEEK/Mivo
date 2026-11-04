package scan

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// run is the state one scan of one library shares with its probe workers: the service, the
// library, the status tracker and the item kind of the library. runV2 adds the placement.
type run struct {
	service  *Service
	library  library.ManagedLibrary
	tracker  *tracker
	itemKind library.ItemKind
}

func (service *Service) newRun(scanned library.ManagedLibrary, scanTracker *tracker) *run {
	return &run{service: service, library: scanned, tracker: scanTracker}
}

// walkError maps a failed walk to the scan failure: a lost root is its own code.
func (scanRun *run) walkError(err error) error {
	err = fmt.Errorf("walk library %s: %w", scanRun.library.ID, err)
	if errors.Is(err, domain.ErrUnavailable) {
		return fail(ErrorCodeRootUnavailable, causeRootUnavailable, err)
	}

	return fail(ErrorCodeFailed, causeWalkFailed, err)
}

func (scanRun *run) logFile(ctx context.Context, message string, job probeJob, attributes ...slog.Attr) {
	attributes = append([]slog.Attr{
		slog.String("library", scanRun.library.ID), slog.String("path", job.state.RelativePath),
	}, attributes...)

	scanRun.service.logger.LogAttrs(ctx, slog.LevelDebug, message, attributes...)
}

package stream

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/adapters/http/statuswriter"
	"github.com/magpeek/mivo/internal/domain"
)

// Trickplay sprite sheets never change once generated (ADR 0030 section 8); a regenerated
// file gets new sheet URLs through a new manifest, so a day of caching is safe.
const (
	trickplaySuffix      = ".jpg"
	trickplayContentType = "image/jpeg"
	trickplayCaching     = "private, max-age=86400"
)

// trickplaySheetPattern is the sheet number shape of the contract: decimal, no sign, no
// leading zeros, below 10000.
var trickplaySheetPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,3})$`)

// Trickplay locates the sprite sheets of media files. Implemented by *trickplay.Service
// (app/trickplay), which checks the viewer.
type Trickplay interface {
	// Sheet returns the absolute path of the 0-based sheet. Errors wrap
	// domain.ErrUnauthenticated without a viewer and domain.ErrNotFound for an unknown or
	// hidden file, missing trickplay or an unknown sheet.
	Sheet(ctx context.Context, fileID string, sheet int) (string, error)
}

// serveTrickplaySheet serves one sprite sheet (GET|HEAD /api/v1/media-files/{fileId}/
// trickplay/{sheet}.jpg). The trickplay service decides what exists and what the viewer may
// see; the handler only opens the file it names.
func (handlers *Handlers) serveTrickplaySheet(writer http.ResponseWriter, request *http.Request) {
	response := statuswriter.New(writer)

	number, suffixOK := strings.CutSuffix(request.PathValue(sheetFileParameter), trickplaySuffix)
	fileID, fileOK := fileIDOf(request)

	if !suffixOK || !fileOK || !trickplaySheetPattern.MatchString(number) {
		writeProblem(response, notFoundProblem())

		return
	}

	sheet, err := strconv.Atoi(number)
	if err != nil {
		writeProblem(response, notFoundProblem())

		return
	}

	path, err := handlers.trickplay.Sheet(request.Context(), fileID, sheet)
	if err != nil {
		handlers.failMedia(response, request, err)

		return
	}

	file, err := openRegularFile(path)
	if err != nil {
		handlers.failMedia(response, request, err)

		return
	}

	defer closeFile(file)

	info, err := file.Stat()
	if err != nil {
		handlers.failMedia(response, request, fmt.Errorf("inspect sheet: %w", err))

		return
	}

	header := response.Header()
	header.Set(headerContentType, trickplayContentType)
	header.Set(headerCacheControl, trickplayCaching)
	http.ServeContent(response, request, "", info.ModTime(), file)
}

// openRegularFile opens path for reading. A missing file or a directory wraps
// domain.ErrNotFound: the manifest may list a sheet FFmpeg has not finished or that was
// pruned meanwhile.
func openRegularFile(path string) (*os.File, error) {
	file, err := os.Open(path) //nolint:gosec // path comes from the trickplay service and lies in the data directory
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("open sheet: %w: %w", domain.ErrNotFound, err)
	}

	if err != nil {
		return nil, fmt.Errorf("open sheet: %w", err)
	}

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		closeFile(file)

		return nil, fmt.Errorf("sheet is no regular file: %w", domain.ErrNotFound)
	}

	return file, nil
}

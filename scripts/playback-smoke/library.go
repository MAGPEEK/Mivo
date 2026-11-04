package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

const (
	librariesPath = "/api/v1/libraries"
	itemsPath     = "/api/v1/items/"
	systemPath    = "/api/v1/system/info"
	// itemPageSize is the largest page the contract allows, so few requests list a library.
	itemPageSize = 500
)

var (
	errNoLibraries   = errors.New("the server has no library and offers no available folder root to create one over (set MIVO_BROWSE_ROOTS, or MIVO_LIBRARIES as a seed)")
	errScanFailed    = errors.New("library scan failed")
	errAmbiguousFile = errors.New("file name is not unique across the libraries")
	errFileNotFound  = errors.New("no catalogued file with this name")
)

func (c *apiClient) systemInfo(ctx context.Context) (apigen.SystemInfo, error) {
	var info apigen.SystemInfo

	err := c.call(ctx, apiCall{method: http.MethodGet, reference: systemPath, want: http.StatusOK, out: &info})

	return info, err
}

// scanLibraries starts a scan of every configured library and waits until all of them are
// finished. It returns the libraries as they are after the scans.
func scanLibraries(ctx context.Context, client *apiClient, poll pollSettings) ([]apigen.Library, error) {
	list, err := listOrSeedLibraries(ctx, client)
	if err != nil {
		return nil, err
	}

	for _, library := range list.Libraries {
		reference := librariesPath + "/" + library.ID.String() + "/scan"

		if err := client.call(ctx, apiCall{method: http.MethodPost, reference: reference, want: http.StatusAccepted}); err != nil {
			return nil, fmt.Errorf("start the scan of library %s: %w", library.Name, err)
		}
	}

	scanned := make([]apigen.Library, 0, len(list.Libraries))

	for _, library := range list.Libraries {
		finished, err := waitForScan(ctx, client, library.ID, poll)
		if err != nil {
			return nil, fmt.Errorf("library %s: %w", library.Name, err)
		}

		scanned = append(scanned, finished)
	}

	return scanned, nil
}

// listOrSeedLibraries lists the libraries; a server without any gets one over its browse
// root (seedLibrary) and is listed again.
func listOrSeedLibraries(ctx context.Context, client *apiClient) (apigen.LibraryList, error) {
	var list apigen.LibraryList

	for attempt := 0; attempt < 2; attempt++ {
		list = apigen.LibraryList{}

		err := client.call(ctx, apiCall{method: http.MethodGet, reference: librariesPath, want: http.StatusOK, out: &list})
		if err != nil {
			return list, fmt.Errorf("list the libraries: %w", err)
		}

		if len(list.Libraries) > 0 {
			return list, nil
		}

		if attempt == 0 {
			if err := seedLibrary(ctx, client); err != nil {
				return list, err
			}
		}
	}

	return list, errNoLibraries
}

// waitForScan polls the library until no scan runs. The scan was started before, and the
// server answers the start only after the scan is registered as running, so the first
// state other than running is the result of this scan.
func waitForScan(ctx context.Context, client *apiClient, id uuid.UUID, poll pollSettings) (apigen.Library, error) {
	for {
		var library apigen.Library

		reference := librariesPath + "/" + id.String()
		if err := client.call(ctx, apiCall{method: http.MethodGet, reference: reference, want: http.StatusOK, out: &library}); err != nil {
			return apigen.Library{}, fmt.Errorf("poll the scan: %w", err)
		}

		switch library.Scan.State {
		case apigen.ScanStateRunning:
		case apigen.ScanStateFailed:
			return apigen.Library{}, fmt.Errorf("%w: %s", errScanFailed, valueOr(library.Scan.ErrorCode, "no error code"))
		case apigen.ScanStateIdle:
			return library, nil
		default:
			// A state a later server version adds is no running scan (the contract asks
			// clients to tolerate new values).
			return library, nil
		}

		if err := poll.wait(ctx, 0); err != nil {
			return apigen.Library{}, fmt.Errorf("wait for the scan: %w", err)
		}
	}
}

// fileIndex maps the file name of every catalogued media file to its ID. Names that occur
// more than once map to the nil UUID, so a lookup can tell "ambiguous" from "missing".
type fileIndex map[string]uuid.UUID

func (index fileIndex) lookup(name string) (uuid.UUID, error) {
	id, found := index[name]

	switch {
	case !found:
		return uuid.UUID{}, fmt.Errorf("%w: %s", errFileNotFound, name)
	case id == uuid.Nil():
		return uuid.UUID{}, fmt.Errorf("%w: %s", errAmbiguousFile, name)
	default:
		return id, nil
	}
}

func (index fileIndex) add(name string, id uuid.UUID) {
	if _, found := index[name]; found {
		index[name] = uuid.Nil()

		return
	}

	index[name] = id
}

// indexFiles pages through the items of every library and reads each item's files.
func indexFiles(ctx context.Context, client *apiClient, libraries []apigen.Library) (fileIndex, error) {
	index := fileIndex{}

	for _, library := range libraries {
		items, err := libraryItems(ctx, client, library.ID)
		if err != nil {
			return nil, fmt.Errorf("list the items of library %s: %w", library.Name, err)
		}

		for _, item := range items {
			var detail apigen.ItemDetail

			reference := itemsPath + item.ID.String()
			if err := client.call(ctx, apiCall{method: http.MethodGet, reference: reference, want: http.StatusOK, out: &detail}); err != nil {
				return nil, fmt.Errorf("read item %q: %w", item.Title, err)
			}

			for _, file := range detail.Files {
				index.add(file.FileName, file.ID)
			}
		}
	}

	return index, nil
}

func libraryItems(ctx context.Context, client *apiClient, id uuid.UUID) ([]apigen.ItemSummary, error) {
	var (
		items  []apigen.ItemSummary
		cursor string
	)

	for {
		query := url.Values{"limit": {strconv.Itoa(itemPageSize)}}
		if cursor != "" {
			query.Set("cursor", cursor)
		}

		var page apigen.ItemPage

		reference := librariesPath + "/" + id.String() + "/items?" + query.Encode()
		if err := client.call(ctx, apiCall{method: http.MethodGet, reference: reference, want: http.StatusOK, out: &page}); err != nil {
			return nil, err
		}

		items = append(items, page.Items...)

		if page.NextCursor == nil || *page.NextCursor == "" {
			return items, nil
		}

		cursor = *page.NextCursor
	}
}

func valueOr(value *string, fallback string) string {
	if value == nil || *value == "" {
		return fallback
	}

	return *value
}

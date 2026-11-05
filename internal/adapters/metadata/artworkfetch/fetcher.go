package artworkfetch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/magpeek/mivo/internal/adapters/netaccess/outbound"
	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
)

// acceptHeader asks only for the formats that are stored.
const acceptHeader = "image/jpeg, image/png, image/webp"

// Getter performs a GET request; *outbound.Client satisfies it. Its allow-list, redirect
// hosts, size cap and SSRF guard apply in addition to the checks of this package.
type Getter interface {
	Get(ctx context.Context, rawURL string, header http.Header) (*outbound.Response, error)
}

// URLBuilder turns a provider reference into the URL to download, for example the TMDB
// adapter's image URL with the size chosen per kind. The result is still checked against
// the allow-list.
type URLBuilder func(ref enrich.ArtworkRef) (string, error)

// Provider is the client, URL builder and allow-list for one artwork provider. A nil
// Client disables the provider: its references fail with domain.ErrUnavailable.
type Provider struct {
	Client Getter
	// ImageURL builds the download URL. Required for TMDB (wired by M6-14 from the TMDB
	// adapter); nil for Cover Art Archive uses the reference path, which is a URL there.
	ImageURL URLBuilder
	// AllowedHosts are exact host names (TMDBHosts, CoverArtHosts in production).
	AllowedHosts []string
}

// Options configures a Fetcher.
type Options struct {
	// DataDir is the absolute data directory; pictures go to <DataDir>/artwork.
	DataDir string
	// TMDB fetches TMDB file paths ("/abc.jpg") through TMDB.ImageURL.
	TMDB Provider
	// CoverArt fetches Cover Art Archive URLs.
	CoverArt Provider
	// AllowInsecureLoopback accepts plain HTTP picture URLs. It exists for tests against
	// httptest servers and must never be set in production wiring.
	AllowInsecureLoopback bool
}

// Fetcher downloads provider pictures into the data directory. It implements
// enrich.ArtworkDownloader and is safe for concurrent use.
type Fetcher struct {
	opts  Options
	store pictureStore
	locks keyLocks
}

var _ enrich.ArtworkDownloader = (*Fetcher)(nil)

// New returns a Fetcher; DataDir must be an absolute path.
func New(opts Options) (*Fetcher, error) {
	if !filepath.IsAbs(opts.DataDir) {
		return nil, fmt.Errorf("artwork fetcher data directory %q is not absolute: %w", opts.DataDir, domain.ErrInvalid)
	}

	return &Fetcher{opts: opts, store: pictureStore{dataDir: opts.DataDir}}, nil
}

// Fetch downloads the picture ref points to unless it is stored already, validates it and
// stores it as artwork/<tag>.<ext>. Invalid references and pictures fail with
// domain.ErrInvalid, hosts outside the allow-list with domain.ErrForbidden, a missing
// picture with domain.ErrNotFound and network trouble with domain.ErrUnavailable.
func (fetcher *Fetcher) Fetch(ctx context.Context, ref enrich.ArtworkRef) (enrich.Stored, error) {
	target, client, err := fetcher.resolve(ref)
	if err != nil {
		return enrich.Stored{}, fmt.Errorf("fetch %s artwork: %w", ref.Provider, err)
	}

	tag := refTag(ref)

	unlock, err := fetcher.locks.lock(ctx, tag)
	if err != nil {
		return enrich.Stored{}, fmt.Errorf("fetch %s artwork: %w", ref.Provider, err)
	}
	defer unlock()

	file, found, err := fetcher.store.find(tag)
	if err == nil && !found {
		file, err = fetcher.download(ctx, client, target, tag)
	}

	if err != nil {
		return enrich.Stored{}, fmt.Errorf("fetch %s artwork: %w", ref.Provider, err)
	}

	return enrich.Stored{DataPath: file.DataPath, Size: file.Size, ModifiedAt: file.ModifiedAt}, nil
}

// Remove deletes stored provider pictures, for example on "Remove match". The caller passes
// only data paths no other item still references, since one picture can serve several
// items. Paths this package did not write are refused with domain.ErrInvalid; missing files
// are ignored.
func (fetcher *Fetcher) Remove(ctx context.Context, dataPaths ...string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("remove provider artwork: %w", err)
	}

	if err := fetcher.store.remove(dataPaths); err != nil {
		return fmt.Errorf("remove provider artwork: %w", err)
	}

	return nil
}

// resolve turns a reference into a checked URL and the client of its provider.
func (fetcher *Fetcher) resolve(ref enrich.ArtworkRef) (string, Getter, error) {
	var provider Provider

	switch ref.Provider {
	case enrich.ArtworkProviderTMDB:
		// The TMDB adapter validates its paths too; this is the second check.
		if err := checkTMDBPath(ref.Path); err != nil {
			return "", nil, err
		}

		provider = fetcher.opts.TMDB
	case enrich.ArtworkProviderCoverArt:
		provider = fetcher.opts.CoverArt
		if provider.ImageURL == nil {
			provider.ImageURL = pathAsURL
		}
	default:
		return "", nil, fmt.Errorf("unknown artwork provider %q: %w", ref.Provider, domain.ErrInvalid)
	}

	if provider.Client == nil || provider.ImageURL == nil {
		return "", nil, fmt.Errorf("provider disabled: %w", domain.ErrUnavailable)
	}

	rawURL, err := provider.ImageURL(ref)
	if err != nil {
		return "", nil, fmt.Errorf("build picture url: %w", err)
	}

	target, err := checkedURL(rawURL, provider.AllowedHosts, fetcher.opts.AllowInsecureLoopback)
	if err != nil {
		return "", nil, err
	}

	return target, provider.Client, nil
}

// download fetches, validates and stores one picture.
func (fetcher *Fetcher) download(ctx context.Context, client Getter, target, tag string) (storedFile, error) {
	body, err := get(ctx, client, target)
	if err != nil {
		return storedFile{}, err
	}

	info, err := validateImage(body)
	if err != nil {
		return storedFile{}, fmt.Errorf("validate %s: %w", outbound.RedactURL(target), err)
	}

	return fetcher.store.write(tag, info.Extension, body)
}

func get(ctx context.Context, client Getter, target string) ([]byte, error) {
	resp, err := client.Get(ctx, target, http.Header{"Accept": {acceptHeader}})
	if errors.Is(err, outbound.ErrBodyTooLarge) {
		return nil, fmt.Errorf("picture exceeds %d bytes: %w", MaxImageBytes, domain.ErrInvalid)
	}

	if err != nil {
		return nil, fmt.Errorf("download picture: %w", err)
	}

	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return nil, fmt.Errorf("download %s: status %d: %w", outbound.RedactURL(target), resp.StatusCode, domain.ErrNotFound)
	case resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices:
		return nil, fmt.Errorf("download %s: status %d: %w", outbound.RedactURL(target), resp.StatusCode, domain.ErrUnavailable)
	}

	return resp.Body, nil
}

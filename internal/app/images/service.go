package images

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

const (
	// maxConcurrentRenders bounds the renders running at a time server-wide, so a page full
	// of new posters cannot starve playback of CPU (ADR 0027 §8).
	maxConcurrentRenders = 2
	// renderTimeout bounds one render including the wait for a free render slot.
	renderTimeout = 15 * time.Second
)

// Request asks for one artwork in one width. Tag is the tag the client used in its URL.
type Request struct {
	ID    string
	Width artwork.Width
	Tag   string
}

// Image is a rendered picture ready to be served. The caller closes Content. ETag is the
// complete header value including quotes; Immutable is true only when the request named the
// current tag, so the response may be cached for good.
type Image struct {
	Content     io.ReadSeekCloser
	Size        int64
	ContentType string
	ETag        string
	Immutable   bool
}

// Dependencies are the ports the service needs. Logger may be nil. Photos and
// PhotoRenderer are optional together: without them the service serves artwork only and a
// photo item ID is unknown. ArtworkDir is the absolute <data>/artwork directory the
// downloaded provider pictures live in (ADR 0031 §11); Resolver confines them to it like
// library files to their root. Empty refuses data-directory pictures.
type Dependencies struct {
	Source        ArtworkSource
	Resolver      PathResolver
	Renderer      Renderer
	Cache         Cache
	Logger        *slog.Logger
	Photos        PhotoSource
	PhotoRenderer Renderer
	ArtworkDir    string
}

// renderTarget is everything a render needs, whatever the ID named: the cache key, where
// the picture is read from and the renderer that decodes it.
type renderTarget struct {
	key         Key
	libraryRoot string
	location    artwork.Location
	renderer    Renderer
}

// Service serves rendered artwork.
type Service struct {
	dependencies Dependencies
	flights      *flightGroup
}

// NewService returns a service using the given ports.
func NewService(dependencies Dependencies) *Service {
	return newServiceWithLimits(dependencies, maxConcurrentRenders, renderTimeout)
}

func newServiceWithLimits(dependencies Dependencies, maxConcurrent int, timeout time.Duration) *Service {
	if dependencies.Logger == nil {
		dependencies.Logger = slog.New(slog.DiscardHandler)
	}

	return &Service{dependencies: dependencies, flights: newFlightGroup(maxConcurrent, timeout)}
}

// Image returns the artwork or photo thumbnail of request rendered in the requested width,
// from the cache when possible. Errors wrap domain.ErrNotFound for an unknown ID or vanished
// source, domain.ErrUnsupported for an undecodable source and domain.ErrUnavailable when
// rendering is not possible right now.
func (service *Service) Image(ctx context.Context, request Request) (Image, error) {
	target, err := service.target(ctx, request)
	if err != nil {
		return Image{}, fmt.Errorf("image %s: %w", request.ID, err)
	}

	key := target.key
	image := Image{
		ContentType: key.Format.ContentType(),
		ETag:        `"` + key.Tag + "-" + request.Width.String() + `"`,
		Immutable:   request.Tag == key.Tag,
	}

	cached, err := service.dependencies.Cache.Open(ctx, key)
	if err == nil {
		image.Content, image.Size = cached.Content, cached.Size

		return image, nil
	}

	if !errors.Is(err, domain.ErrNotFound) {
		return Image{}, fmt.Errorf("image %s: open cache: %w", request.ID, err)
	}

	// Photos share the flight group, so photo thumbnails and artwork together stay within
	// the server-wide render limit.
	data, err := service.flights.do(ctx, key, func(renderCtx context.Context) ([]byte, error) {
		return service.renderAndStore(renderCtx, target)
	})
	if err != nil {
		return Image{}, fmt.Errorf("image %s width %s: %w", request.ID, request.Width, err)
	}

	image.Content, image.Size = nopCloser{bytes.NewReader(data)}, int64(len(data))

	return image, nil
}

// renderAndStore renders one picture and stores it. A failed store is logged only: the
// picture is still served and rendered again on the next request.
func (service *Service) renderAndStore(ctx context.Context, target renderTarget) ([]byte, error) {
	path, err := service.sourcePath(target)
	if err != nil {
		return nil, err
	}

	key := target.key

	rendered, err := target.renderer.Render(ctx, artwork.RenderRequest{
		AbsolutePath: path,
		StreamIndex:  target.location.StreamIndex,
		MaxWidth:     key.Width,
		Format:       key.Format,
	})
	if err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}

	if storeErr := service.dependencies.Cache.Store(ctx, key, rendered.Data); storeErr != nil {
		service.dependencies.Logger.WarnContext(ctx, "storing rendered image failed",
			slog.String("artwork_id", key.ID), slog.Any("error", storeErr))
	}

	return rendered.Data, nil
}

// DataArtworkDirectory is the folder of the data directory provider pictures are stored in
// (ADR 0031 §11); their data paths are "artwork/<file>". It matches the artwork fetcher's
// directory, which the wiring checks.
const DataArtworkDirectory = "artwork"

// sourcePath resolves the file a picture is read from. It is the one place that switches on
// the source location: a data-directory picture resolves below ArtworkDir only, so a stored
// path can never reach the database or anything else in the data directory.
func (service *Service) sourcePath(target renderTarget) (string, error) {
	root, relativePath := target.libraryRoot, target.location.RelativePath

	if dataPath := target.location.DataPath; dataPath != "" {
		name, inArtwork := strings.CutPrefix(dataPath, DataArtworkDirectory+"/")
		if service.dependencies.ArtworkDir == "" || !inArtwork {
			return "", fmt.Errorf("resolve data-directory picture %q: %w", dataPath, domain.ErrUnsupported)
		}

		root, relativePath = service.dependencies.ArtworkDir, name
	}

	path, err := service.dependencies.Resolver.Resolve(root, relativePath)
	if err != nil {
		return "", fmt.Errorf("resolve %q below %q: %w", relativePath, root, err)
	}

	return path, nil
}

// target looks the ID of request up: artwork first, then, with a photo source, a photo
// item. An ID neither knows wraps domain.ErrNotFound.
func (service *Service) target(ctx context.Context, request Request) (renderTarget, error) {
	stored, err := service.dependencies.Source.Artwork(ctx, request.ID)
	if err == nil {
		return renderTarget{
			key: Key{
				ID: stored.ID, Tag: stored.Tag, Width: request.Width, Format: artwork.OutputFormat(stored.Kind),
			},
			libraryRoot: stored.LibraryRoot,
			location:    stored.Location,
			renderer:    service.dependencies.Renderer,
		}, nil
	}

	if !errors.Is(err, domain.ErrNotFound) || service.dependencies.Photos == nil {
		return renderTarget{}, err
	}

	file, err := service.dependencies.Photos.PhotoFile(ctx, request.ID)
	if err != nil {
		return renderTarget{}, fmt.Errorf("photo: %w", err)
	}

	if service.dependencies.PhotoRenderer == nil {
		return renderTarget{}, fmt.Errorf("photo: no photo renderer configured: %w", domain.ErrUnavailable)
	}

	return photoTarget(file, request.Width, service.dependencies.PhotoRenderer), nil
}

// Purge removes cached renderings of artwork that no longer exists or changed. It is safe to
// run while images are served: an open rendering stays readable after its removal on
// Unix-like systems, and the cache retries locked files on Windows.
func (service *Service) Purge(ctx context.Context) error {
	tags, err := service.dependencies.Source.ArtworkTags(ctx)
	if err != nil {
		return fmt.Errorf("purge image cache: read tags: %w", err)
	}

	photoTags, err := service.photoTags(ctx)
	if err != nil {
		return fmt.Errorf("purge image cache: %w", err)
	}

	keep := func(id, tag string) bool {
		if current, ok := tags[id]; ok {
			return current == tag
		}

		current, ok := photoTags[id]

		return ok && current == tag
	}

	if err := service.dependencies.Cache.Purge(ctx, keep); err != nil {
		return fmt.Errorf("purge image cache: %w", err)
	}

	return nil
}

// nopCloser gives an in-memory reader the Close the cache files have.
type nopCloser struct {
	*bytes.Reader
}

// Close does nothing.
func (nopCloser) Close() error {
	return nil
}

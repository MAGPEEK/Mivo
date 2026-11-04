package images

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
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

// Dependencies are the ports the service needs. Logger may be nil.
type Dependencies struct {
	Source   ArtworkSource
	Resolver PathResolver
	Renderer Renderer
	Cache    Cache
	Logger   *slog.Logger
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

// Image returns the artwork of request rendered in the requested width, from the cache when
// possible. Errors wrap domain.ErrNotFound for an unknown artwork or vanished source,
// domain.ErrUnsupported for an undecodable source and domain.ErrUnavailable when rendering
// is not possible right now.
func (service *Service) Image(ctx context.Context, request Request) (Image, error) {
	stored, err := service.dependencies.Source.Artwork(ctx, request.ID)
	if err != nil {
		return Image{}, fmt.Errorf("image %s: %w", request.ID, err)
	}

	key := Key{ID: stored.ID, Tag: stored.Tag, Width: request.Width, Format: artwork.OutputFormat(stored.Kind)}
	image := Image{
		ContentType: key.Format.ContentType(),
		ETag:        `"` + stored.Tag + "-" + request.Width.String() + `"`,
		Immutable:   request.Tag == stored.Tag,
	}

	cached, err := service.dependencies.Cache.Open(ctx, key)
	if err == nil {
		image.Content, image.Size = cached.Content, cached.Size

		return image, nil
	}

	if !errors.Is(err, domain.ErrNotFound) {
		return Image{}, fmt.Errorf("image %s: open cache: %w", request.ID, err)
	}

	data, err := service.flights.do(ctx, key, func(renderCtx context.Context) ([]byte, error) {
		return service.renderAndStore(renderCtx, stored, key)
	})
	if err != nil {
		return Image{}, fmt.Errorf("image %s width %s: %w", request.ID, request.Width, err)
	}

	image.Content, image.Size = nopCloser{bytes.NewReader(data)}, int64(len(data))

	return image, nil
}

// renderAndStore renders one picture and stores it. A failed store is logged only: the
// picture is still served and rendered again on the next request.
func (service *Service) renderAndStore(ctx context.Context, stored artwork.Stored, key Key) ([]byte, error) {
	path, err := service.sourcePath(stored)
	if err != nil {
		return nil, err
	}

	rendered, err := service.dependencies.Renderer.Render(ctx, artwork.RenderRequest{
		AbsolutePath: path,
		StreamIndex:  stored.Location.StreamIndex,
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

// sourcePath resolves the file a picture is read from. It is the one place that switches on
// the source location: data-directory pictures (milestone 6) need their own resolver.
func (service *Service) sourcePath(stored artwork.Stored) (string, error) {
	if stored.Location.DataPath != "" {
		return "", fmt.Errorf("resolve data-directory picture %q: %w", stored.Location.DataPath, domain.ErrUnsupported)
	}

	path, err := service.dependencies.Resolver.Resolve(stored.LibraryRoot, stored.Location.RelativePath)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", stored.Location.RelativePath, err)
	}

	return path, nil
}

// Purge removes cached renderings of artwork that no longer exists or changed. It is safe to
// run while images are served: an open rendering stays readable after its removal on
// Unix-like systems, and the cache retries locked files on Windows.
func (service *Service) Purge(ctx context.Context) error {
	tags, err := service.dependencies.Source.ArtworkTags(ctx)
	if err != nil {
		return fmt.Errorf("purge image cache: read tags: %w", err)
	}

	keep := func(id, tag string) bool {
		current, ok := tags[id]

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

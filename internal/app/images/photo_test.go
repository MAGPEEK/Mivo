package images_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/images"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

const (
	photoID      = "3b241101-e2bb-4255-8caf-4136c566a962"
	photoPath    = "Holiday/IMG_0001.jpg"
	photoSize    = 2_000_000
	unknownID    = "9a8b7c6d-0000-4000-8000-000000000000"
	photoLibrary = "/photos"
)

// fakePhotos is an in-memory PhotoSource.
type fakePhotos struct {
	files   map[string]images.PhotoFile
	tagsErr error
}

func (photos *fakePhotos) PhotoFile(_ context.Context, itemID string) (images.PhotoFile, error) {
	file, ok := photos.files[itemID]
	if !ok {
		return images.PhotoFile{}, fmt.Errorf("photo %s: %w", itemID, domain.ErrNotFound)
	}

	return file, nil
}

func (photos *fakePhotos) PhotoTags(context.Context) (map[string]string, error) {
	if photos.tagsErr != nil {
		return nil, photos.tagsErr
	}

	tags := make(map[string]string, len(photos.files))
	for id, file := range photos.files {
		tags[id] = file.Tag()
	}

	return tags, nil
}

type photoFixture struct {
	*fixture
	photos        *fakePhotos
	photoRenderer *fakeRenderer
}

func newPhotoFixture(t *testing.T) *photoFixture {
	t.Helper()

	base := newFixture(t, &fakeRenderer{})
	photos := &fakePhotos{files: map[string]images.PhotoFile{
		photoID: {
			ItemID: photoID, LibraryRoot: photoLibrary, RelativePath: photoPath, Size: photoSize,
			ModifiedAt: time.UnixMilli(1_700_000_000_000),
		},
	}}
	photoRenderer := &fakeRenderer{}
	base.service = images.NewService(images.Dependencies{
		Source: base.source, Resolver: prefixResolver{}, Renderer: base.renderer, Cache: base.cache,
		Photos: photos, PhotoRenderer: photoRenderer,
	})

	return &photoFixture{fixture: base, photos: photos, photoRenderer: photoRenderer}
}

func (fixture *photoFixture) photoTag() string {
	return fixture.photos.files[photoID].Tag()
}

func TestImageRendersAPhotoFromItsMainFile(t *testing.T) {
	t.Parallel()

	fixture := newPhotoFixture(t)
	tag := fixture.photoTag()

	image, err := fixture.service.Image(t.Context(), images.Request{ID: photoID, Width: artwork.Width480, Tag: tag})
	if err != nil {
		t.Fatalf("Image: %v", err)
	}

	if got := readAll(t, image); got != "img:/photos/"+photoPath {
		t.Errorf("content = %q", got)
	}

	if image.ContentType != artwork.FormatJPEG.ContentType() || !image.Immutable || image.ETag != `"`+tag+`-480"` {
		t.Errorf("image = %+v", image)
	}

	if fixture.renderer.calls() != 0 || fixture.photoRenderer.calls() != 1 {
		t.Fatalf("artwork renders = %d, photo renders = %d; want 0 and 1",
			fixture.renderer.calls(), fixture.photoRenderer.calls())
	}

	request := fixture.photoRenderer.requests[0]
	if request.StreamIndex != artwork.WholeFile || request.MaxWidth != artwork.Width480 || request.Format != artwork.FormatJPEG {
		t.Errorf("render request = %+v", request)
	}
}

func TestPhotoCacheKeyFollowsModificationTimeAndWidth(t *testing.T) {
	t.Parallel()

	fixture := newPhotoFixture(t)

	render := func(width artwork.Width) {
		t.Helper()

		image, err := fixture.service.Image(t.Context(), images.Request{ID: photoID, Width: width})
		if err != nil {
			t.Fatalf("Image: %v", err)
		}

		_ = readAll(t, image)
	}

	render(artwork.Width320)
	render(artwork.Width320)
	render(artwork.Width640)

	if calls := fixture.photoRenderer.calls(); calls != 2 {
		t.Fatalf("renders = %d, want 2 (one per width)", calls)
	}

	file := fixture.photos.files[photoID]
	file.ModifiedAt = file.ModifiedAt.Add(time.Minute)
	fixture.photos.files[photoID] = file

	render(artwork.Width320)

	if calls := fixture.photoRenderer.calls(); calls != 3 {
		t.Errorf("renders after the edit = %d, want 3", calls)
	}
}

func TestPhotoLookupErrors(t *testing.T) {
	t.Parallel()

	fixture := newPhotoFixture(t)
	if _, err := fixture.service.Image(t.Context(), images.Request{ID: unknownID, Width: artwork.Width160}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown ID = %v, want ErrNotFound", err)
	}

	fixture.photoRenderer.err = fmt.Errorf("decode: %w", domain.ErrUnsupported)
	if _, err := fixture.service.Image(t.Context(), images.Request{ID: photoID, Width: artwork.Width160}); !errors.Is(err, domain.ErrUnsupported) {
		t.Errorf("undecodable photo = %v, want ErrUnsupported", err)
	}

	withoutPhotos := newFixture(t, &fakeRenderer{})
	if _, err := withoutPhotos.service.Image(t.Context(), images.Request{ID: photoID, Width: artwork.Width160}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("photo without photo source = %v, want ErrNotFound", err)
	}

	withoutRenderer := images.NewService(images.Dependencies{
		Source: fixture.source, Resolver: prefixResolver{}, Renderer: fixture.renderer, Cache: newMemoryCache(),
		Photos: fixture.photos,
	})
	if _, err := withoutRenderer.Image(t.Context(), images.Request{ID: photoID, Width: artwork.Width160}); !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("photo without renderer = %v, want ErrUnavailable", err)
	}
}

func TestPhotoLookupKeepsArtworkSourceFailures(t *testing.T) {
	t.Parallel()

	fixture := newPhotoFixture(t)
	failure := errors.New("database is locked")
	service := images.NewService(images.Dependencies{
		Source: failingSource{err: failure}, Resolver: prefixResolver{}, Renderer: fixture.renderer,
		Cache: fixture.cache, Photos: fixture.photos, PhotoRenderer: fixture.photoRenderer,
	})

	if _, err := service.Image(t.Context(), images.Request{ID: photoID, Width: artwork.Width160}); !errors.Is(err, failure) {
		t.Errorf("Image = %v, want the artwork source failure", err)
	}

	if fixture.photoRenderer.calls() != 0 {
		t.Error("a photo was rendered although the artwork lookup failed")
	}
}

func TestPurgeKeepsCurrentPhotoThumbnails(t *testing.T) {
	t.Parallel()

	fixture := newPhotoFixture(t)
	current := images.Key{ID: photoID, Tag: fixture.photoTag(), Width: artwork.Width320, Format: artwork.FormatJPEG}
	stale := images.Key{ID: photoID, Tag: "0000000000000000", Width: artwork.Width320, Format: artwork.FormatJPEG}
	poster := images.Key{ID: posterID, Tag: posterTag, Width: artwork.Width320, Format: artwork.FormatJPEG}

	for _, key := range []images.Key{current, stale, poster} {
		if err := fixture.cache.Store(t.Context(), key, []byte("x")); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}

	if err := fixture.service.Purge(t.Context()); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	if fixture.cache.size() != 2 {
		t.Errorf("entries after purge = %d, want 2 (current photo and poster)", fixture.cache.size())
	}

	if _, err := fixture.cache.Open(t.Context(), stale); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("stale photo thumbnail survived: %v", err)
	}

	fixture.photos.tagsErr = errors.New("database is locked")
	if err := fixture.service.Purge(t.Context()); !errors.Is(err, fixture.photos.tagsErr) {
		t.Errorf("Purge with failing photo tags = %v", err)
	}
}

// failingSource fails every artwork lookup with err.
type failingSource struct {
	err error
}

func (source failingSource) Artwork(context.Context, string) (artwork.Stored, error) {
	return artwork.Stored{}, source.err
}

func (source failingSource) ArtworkTags(context.Context) (map[string]string, error) {
	return nil, source.err
}

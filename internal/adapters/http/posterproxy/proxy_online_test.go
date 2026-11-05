package posterproxy_test

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/posterproxy"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/app/enrich"
)

// TestServePosterOpensNoConnectionWhenSwitchedOff: with online metadata or TMDB off a held
// picture is still served, but nothing is downloaded (ADR 0031 §2).
func TestServePosterOpensNoConnectionWhenSwitchedOff(t *testing.T) {
	t.Parallel()

	for name, off := range map[string][2]bool{"online off": {false, true}, "tmdb off": {true, false}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			source := &fakeSource{body: readImage(t, "poster.jpg")}
			settings := newSwitches(true, true)
			proxy, handler, _ := newProxy(t, posterproxy.Options{Source: source, Settings: settings})

			held := proxy.PosterURL(poster("/held.jpg"))
			if recorder := get(t, handler, held); recorder.Code != http.StatusOK {
				t.Fatalf("first answer = %d", recorder.Code)
			}

			settings.set(off[0], off[1])

			if recorder := get(t, handler, held); recorder.Code != http.StatusOK {
				t.Errorf("held picture while switched off = %d, want 200 from memory", recorder.Code)
			}

			recorder := get(t, handler, proxy.PosterURL(poster("/new.jpg")))
			assertProblem(t, recorder, http.StatusConflict, restapi.CodeOnlineDisabled)

			if recorder.Header().Get("Retry-After") != "" {
				t.Error("a switched-off provider answered Retry-After")
			}

			if source.callCount() != 1 {
				t.Errorf("previews = %d, want only the one before switching off", source.callCount())
			}
		})
	}
}

func TestServePosterFailsWhenTheSettingsCannotBeRead(t *testing.T) {
	t.Parallel()

	source := &fakeSource{body: readImage(t, "poster.jpg")}
	settings := newSwitches(true, true)
	settings.err = errors.New("settings store gone")
	proxy, handler, _ := newProxy(t, posterproxy.Options{Source: source, Settings: settings})

	assertProblem(t, get(t, handler, proxy.PosterURL(poster("/a.jpg"))), http.StatusInternalServerError, restapi.CodeInternal)

	if source.callCount() != 0 {
		t.Errorf("previews = %d, want none", source.callCount())
	}
}

// blockingSource holds every preview until release is closed and counts the calls.
type blockingSource struct {
	body    []byte
	started chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (source *blockingSource) Preview(context.Context, enrich.ArtworkRef) ([]byte, error) {
	if source.calls.Add(1) == 1 {
		close(source.started)
	}

	<-source.release

	return source.body, nil
}

func TestConcurrentRequestsForOnePosterFetchOnce(t *testing.T) {
	t.Parallel()

	const requests = 16

	source := &blockingSource{body: readImage(t, "poster.jpg"), started: make(chan struct{}), release: make(chan struct{})}
	proxy, handler, _ := newProxy(t, posterproxy.Options{Source: source})
	target := proxy.PosterURL(poster("/a.jpg"))

	var (
		group sync.WaitGroup
		ok    atomic.Int32
	)

	for range requests {
		group.Go(func() {
			if get(t, handler, target).Code == http.StatusOK {
				ok.Add(1)
			}
		})
	}

	<-source.started
	close(source.release)
	group.Wait()

	if calls := source.calls.Load(); calls != 1 {
		t.Errorf("previews = %d, want exactly 1", calls)
	}

	if ok.Load() != requests {
		t.Errorf("successful answers = %d, want %d", ok.Load(), requests)
	}
}

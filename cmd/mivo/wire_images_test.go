package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	appimages "github.com/magpeek/mivo/internal/app/images"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

// blockingSource is an artwork source whose tag listing waits for the purge to be cancelled.
type blockingSource struct {
	started chan struct{}
}

func (source blockingSource) Artwork(context.Context, string) (artwork.Stored, error) {
	return artwork.Stored{}, nil
}

func (source blockingSource) ArtworkTags(ctx context.Context) (map[string]string, error) {
	close(source.started)
	<-ctx.Done()

	return nil, ctx.Err()
}

// TestImagePurgeInBackgroundStopsWithItsStopFunction: the startup purge must not outlive the
// shutdown step that waits for it, even when the database call hangs.
func TestImagePurgeInBackgroundStopsWithItsStopFunction(t *testing.T) {
	t.Parallel()

	source := blockingSource{started: make(chan struct{})}
	service := appimages.NewService(appimages.Dependencies{Source: source})
	app := application{logger: slog.New(slog.DiscardHandler)}

	stop := app.purgeImageCacheInBackground(t.Context(), service)

	select {
	case <-source.started:
	case <-time.After(scanTimeout):
		t.Fatal("the purge did not start")
	}

	done := make(chan struct{})

	go func() {
		stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(scanTimeout):
		t.Fatal("stop did not return: the purge ignores cancellation")
	}
}

// TestRunAnswersTheImageRouteWithAProblemForOtherMethods: the image mount shares the API's
// 404 and 405 problem answers (M3-09b review).
func TestRunAnswersTheImageRouteWithAProblemForOtherMethods(t *testing.T) {
	t.Parallel()

	server := startTestServer(t, isolatedEnvironment(t), runHooks{})
	address := server.baseURL + "/api/v1/images/7e6d5c4b-3a2f-4e1d-9c0b-a9f8e7d6c5b4?width=320"

	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, address, http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", address, err)
	}

	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxStreamBytes))
	if err != nil {
		t.Fatalf("read the response: %v", err)
	}

	var problem apigen.Problem
	if err := json.Unmarshal(body, &problem); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}

	if response.StatusCode != http.StatusMethodNotAllowed || problem.Code != "method_not_allowed" {
		t.Errorf("POST image = %d %+v, want 405 method_not_allowed", response.StatusCode, problem)
	}

	if allow := response.Header.Get("Allow"); allow != "GET, HEAD" {
		t.Errorf("Allow = %q, want %q", allow, "GET, HEAD")
	}

	server.stop(t)
}

package main

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/adapters/sqlite"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	appsubtitles "github.com/magpeek/mivo/internal/app/subtitles"
	"github.com/magpeek/mivo/internal/app/transcoder"
	apptrickplay "github.com/magpeek/mivo/internal/app/trickplay"
)

// streamingServices are the services of the streaming extension that sit next to the playback
// service: hardware status, WebVTT subtitles, trickplay sheets and the lookups of the session
// list and the subtitle track list (ADR 0030).
type streamingServices struct {
	transcoder *transcoder.Service
	subtitles  *appsubtitles.Service
	trickplay  *apptrickplay.Service
	tracks     externalSubtitleLookup
	items      sessionItems
	sessions   *appplayback.Service
	// detection closes when the startup hardware detection has ended.
	detection <-chan struct{}
	// stopWork cancels the context of the background work.
	stopWork context.CancelFunc
}

// newStreamingServices builds the subtitle and trickplay services on the playback service and
// starts the background work: the hardware detection and the trickplay worker. Nothing here
// delays the listener.
func (app application) newStreamingServices(ctx context.Context, db *sqlite.DB, adapters playbackAdapters, sessions *appplayback.Service) (streamingServices, error) {
	subtitleService, err := app.newSubtitleService(ctx, db, adapters.files, adapters.tools)
	if err != nil {
		return streamingServices{}, err
	}

	trickplayService, err := app.newTrickplayService(db, adapters, sessions)
	if err != nil {
		return streamingServices{}, err
	}

	// The background work stops with the server context or when close cancels its own, so a
	// failed start does not leave a detection running.
	workContext, stopWork := context.WithCancel(ctx)

	if err := trickplayService.Start(workContext); err != nil {
		stopWork()

		return streamingServices{}, fmt.Errorf("start trickplay service: %w", err)
	}

	reader := sqlite.NewCatalogReader(db)

	return streamingServices{
		transcoder: adapters.transcoder, subtitles: subtitleService, trickplay: trickplayService,
		tracks: externalSubtitleLookup{reader: reader}, items: sessionItems{reader: reader}, sessions: sessions,
		detection: adapters.transcoder.Start(workContext), stopWork: stopWork,
	}, nil
}

// api is the optional services of the REST adapter: the session list, the transcoder status,
// the trickplay manifest and the subtitle track list.
func (streaming streamingServices) api() restapi.StreamingServices {
	return restapi.StreamingServices{
		Sessions: streaming.sessions, SessionItems: streaming.items, Transcoder: streaming.transcoder,
		Trickplay: streaming.trickplay, Tracks: streaming.tracks,
	}
}

// close stops the trickplay worker (cancelling a running generation) and the hardware
// detection and waits for both. It does not return before they ended, so the caller bounds it.
func (streaming streamingServices) close() {
	if streaming.stopWork != nil {
		streaming.stopWork()
	}

	if streaming.trickplay != nil {
		streaming.trickplay.Close()
	}

	if streaming.detection != nil {
		<-streaming.detection
	}
}

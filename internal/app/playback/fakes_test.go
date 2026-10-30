package playback_test

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// memoryFiles is an in-memory FileSource.
type memoryFiles struct {
	mutex sync.Mutex
	files map[string]appplayback.PlayableFile
	// err, when set, is returned by every load.
	err   error
	loads int
}

func (source *memoryFiles) PlayableFile(_ context.Context, fileID string) (appplayback.PlayableFile, error) {
	source.mutex.Lock()
	defer source.mutex.Unlock()

	source.loads++

	if source.err != nil {
		return appplayback.PlayableFile{}, source.err
	}

	file, found := source.files[fileID]
	if !found {
		return appplayback.PlayableFile{}, fmt.Errorf("media file %s: %w", fileID, domain.ErrNotFound)
	}

	return file, nil
}

func (source *memoryFiles) loadCount() int {
	source.mutex.Lock()
	defer source.mutex.Unlock()

	return source.loads
}

// joiningResolver resolves by joining root and relative path, or fails with err.
type joiningResolver struct {
	mutex sync.Mutex
	err   error
}

func (resolver *joiningResolver) Resolve(root, relativePath string) (string, error) {
	resolver.mutex.Lock()
	defer resolver.mutex.Unlock()

	if resolver.err != nil {
		return "", resolver.err
	}

	return root + "/" + relativePath, nil
}

// keyframeCall records one keyframe lookup.
type keyframeCall struct {
	path     string
	position time.Duration
}

// scriptedKeyframes answers every lookup with keyframe or err and records the calls.
type scriptedKeyframes struct {
	mutex    sync.Mutex
	keyframe time.Duration
	err      error
	calls    []keyframeCall
}

func (locator *scriptedKeyframes) KeyframeAtOrBefore(_ context.Context, path string, position time.Duration) (time.Duration, error) {
	locator.mutex.Lock()
	defer locator.mutex.Unlock()

	locator.calls = append(locator.calls, keyframeCall{path: path, position: position})

	if locator.err != nil {
		return 0, locator.err
	}

	return locator.keyframe, nil
}

func (locator *scriptedKeyframes) recordedCalls() []keyframeCall {
	locator.mutex.Lock()
	defer locator.mutex.Unlock()

	return append([]keyframeCall(nil), locator.calls...)
}

// fakeProduction is a Production whose readiness the test scripts.
type fakeProduction struct {
	mutex    sync.Mutex
	readyErr error
	stops    int
	done     chan struct{}
	doneOnce sync.Once
}

func newFakeProduction() *fakeProduction {
	return &fakeProduction{done: make(chan struct{})}
}

func (production *fakeProduction) Ready(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	production.mutex.Lock()
	defer production.mutex.Unlock()

	// The port contract: after Stop, Ready reports the stop, not the kill's exit error.
	if production.stops > 0 {
		return fmt.Errorf("production killed: %w", appplayback.ErrProductionStopped)
	}

	return production.readyErr
}

func (production *fakeProduction) Stop() {
	production.mutex.Lock()
	production.stops++
	production.mutex.Unlock()

	production.doneOnce.Do(func() { close(production.done) })
}

func (production *fakeProduction) Done() <-chan struct{} {
	return production.done
}

func (production *fakeProduction) stopCount() int {
	production.mutex.Lock()
	defer production.mutex.Unlock()

	return production.stops
}

func (production *fakeProduction) setReadyErr(err error) {
	production.mutex.Lock()
	defer production.mutex.Unlock()

	production.readyErr = err
}

// startedJob is one call of the fake producer.
type startedJob struct {
	// ctx is kept so tests can check which context the production runs under.
	ctx        context.Context
	job        appplayback.Job
	production *fakeProduction
}

// fakeProducer records every start. When gate is set, each start signals entered and waits
// for gate to close, so tests can hold starts in flight.
type fakeProducer struct {
	mutex   sync.Mutex
	err     error
	started []startedJob
	entered chan struct{}
	gate    chan struct{}
}

func (producer *fakeProducer) Start(ctx context.Context, job appplayback.Job) (appplayback.Production, error) {
	if producer.gate != nil {
		producer.entered <- struct{}{}
		<-producer.gate
	}

	producer.mutex.Lock()
	defer producer.mutex.Unlock()

	if producer.err != nil {
		return nil, producer.err
	}

	production := newFakeProduction()
	producer.started = append(producer.started, startedJob{ctx: ctx, job: job, production: production})

	return production, nil
}

func (producer *fakeProducer) jobs() []startedJob {
	producer.mutex.Lock()
	defer producer.mutex.Unlock()

	return append([]startedJob(nil), producer.started...)
}

// productionOf returns the production started for a session.
func (producer *fakeProducer) productionOf(sessionID string) *fakeProduction {
	for _, started := range producer.jobs() {
		if started.job.SessionID == sessionID {
			return started.production
		}
	}

	return nil
}

// memoryWorkspace records created and removed session directories.
type memoryWorkspace struct {
	mutex     sync.Mutex
	createErr error
	removeErr error
	created   []string
	removed   []string
}

func (workspace *memoryWorkspace) Create(ctx context.Context, sessionID string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	workspace.mutex.Lock()
	defer workspace.mutex.Unlock()

	if workspace.createErr != nil {
		return "", workspace.createErr
	}

	workspace.created = append(workspace.created, sessionID)

	return workspaceDir(sessionID), nil
}

func (workspace *memoryWorkspace) Remove(ctx context.Context, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	workspace.mutex.Lock()
	defer workspace.mutex.Unlock()

	workspace.removed = append(workspace.removed, sessionID)

	return workspace.removeErr
}

func (workspace *memoryWorkspace) createdIDs() []string {
	workspace.mutex.Lock()
	defer workspace.mutex.Unlock()

	return append([]string(nil), workspace.created...)
}

func (workspace *memoryWorkspace) removedIDs() []string {
	workspace.mutex.Lock()
	defer workspace.mutex.Unlock()

	return append([]string(nil), workspace.removed...)
}

func workspaceDir(sessionID string) string {
	return "/transcode/sessions/" + sessionID
}

// pathURLs builds recognisable stream URLs.
type pathURLs struct{}

func (pathURLs) StreamURL(sessionID string, delivery playback.Delivery) string {
	return "/stream/" + sessionID + "/" + string(delivery)
}

// sequenceIDs returns canonical UUIDs numbered from 1, or fails with err. repeat makes it
// return the same ID every time.
type sequenceIDs struct {
	mutex  sync.Mutex
	next   int
	err    error
	repeat bool
}

func (generator *sequenceIDs) NewID() (string, error) {
	generator.mutex.Lock()
	defer generator.mutex.Unlock()

	if generator.err != nil {
		return "", generator.err
	}

	if !generator.repeat {
		generator.next++
	}

	return sessionID(generator.next), nil
}

// sessionID is the ID sequenceIDs returns as its number-th ID.
func sessionID(number int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", number)
}

// manualClock is a clock that only moves when the test advances it.
type manualClock struct {
	mutex sync.Mutex
	now   time.Time
}

func (clock *manualClock) Now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()

	return clock.now
}

func (clock *manualClock) advance(step time.Duration) {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()

	clock.now = clock.now.Add(step)
}

// lockedBuffer collects log output written from several goroutines.
type lockedBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (buffer *lockedBuffer) Write(data []byte) (int, error) {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()

	return buffer.buffer.Write(data)
}

func (buffer *lockedBuffer) String() string {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()

	return buffer.buffer.String()
}

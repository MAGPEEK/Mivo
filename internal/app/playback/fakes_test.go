package playback_test

import (
	"bytes"
	"context"
	"errors"
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

// errNoSegments is what a fake production reports for a clean exit without any segment.
var errNoSegments = errors.New("ffmpeg exited without writing a segment")

// fakeProduction is a Production that keeps the port contract while the test decides when
// the first segment appears, how the process exits and how long Stop takes.
type fakeProduction struct {
	mutex   sync.Mutex
	stops   int
	exitErr error
	// segment is closed once the first segment exists.
	segment     chan struct{}
	segmentOnce sync.Once
	done        chan struct{}
	doneOnce    sync.Once
	// stopGate, when set, makes Stop wait for it to close before the process "exits".
	stopGate <-chan struct{}
	// jobContext is the context the producer got; killedByContext records whether it was
	// already cancelled when Stop ran, which a real producer would report as a crash.
	jobContext      context.Context
	killedByContext bool
}

func newFakeProduction(jobContext context.Context, stopGate <-chan struct{}) *fakeProduction {
	return &fakeProduction{
		segment: make(chan struct{}), done: make(chan struct{}),
		stopGate: stopGate, jobContext: jobContext,
	}
}

func (production *fakeProduction) Ready(ctx context.Context) error {
	// The port contract: once the process has ended, Ready reports how at once.
	select {
	case <-production.done:
		return production.outcome()
	default:
	}

	select {
	case <-production.segment:
		return nil
	case <-production.done:
		return production.outcome()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// outcome is the result of Ready after the process ended, in the order of the port contract.
func (production *fakeProduction) outcome() error {
	production.mutex.Lock()
	defer production.mutex.Unlock()

	switch {
	case production.stops > 0:
		return fmt.Errorf("production killed: %w", appplayback.ErrProductionStopped)
	case production.exitErr != nil:
		return production.exitErr
	case !production.hasSegment():
		return errNoSegments
	default:
		return nil
	}
}

func (production *fakeProduction) hasSegment() bool {
	select {
	case <-production.segment:
		return true
	default:
		return false
	}
}

func (production *fakeProduction) Stop() {
	production.mutex.Lock()
	production.stops++

	if production.jobContext.Err() != nil {
		production.killedByContext = true
	}

	production.mutex.Unlock()

	if production.stopGate != nil {
		<-production.stopGate
	}

	production.doneOnce.Do(func() { close(production.done) })
}

func (production *fakeProduction) Done() <-chan struct{} {
	return production.done
}

// stopCount counts the Stop calls, including those still waiting at the stop gate.
func (production *fakeProduction) stopCount() int {
	production.mutex.Lock()
	defer production.mutex.Unlock()

	return production.stops
}

// stoppedAfterKill reports whether any Stop ran after the job context was cancelled.
func (production *fakeProduction) stoppedAfterKill() bool {
	production.mutex.Lock()
	defer production.mutex.Unlock()

	return production.killedByContext
}

// writeSegment lets the production report its first segment.
func (production *fakeProduction) writeSegment() {
	production.segmentOnce.Do(func() { close(production.segment) })
}

// exit ends the process by itself, with err as its exit error (nil: a clean exit).
func (production *fakeProduction) exit(err error) {
	production.mutex.Lock()
	production.exitErr = err
	production.mutex.Unlock()

	production.doneOnce.Do(func() { close(production.done) })
}

// startedJob is one call of the fake producer.
type startedJob struct {
	// ctx is kept so tests can check which context the production runs under.
	ctx        context.Context
	job        appplayback.Job
	production *fakeProduction
}

// fakeProducer records every start. When gate is set, each start signals entered and waits
// for gate to close, so tests can hold starts in flight. Productions have their first
// segment at once unless holdSegments is set; stopGate is handed to every production.
type fakeProducer struct {
	mutex        sync.Mutex
	err          error
	started      []startedJob
	entered      chan struct{}
	gate         chan struct{}
	holdSegments bool
	stopGate     chan struct{}
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

	production := newFakeProduction(ctx, producer.stopGate)
	if !producer.holdSegments {
		production.writeSegment()
	}

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

// memoryWorkspace records created and removed session directories. Purge reports
// purgeRemoved leftovers and purgeErr; when purgeGate is set, Purge waits for it to close.
type memoryWorkspace struct {
	mutex        sync.Mutex
	createErr    error
	removeErr    error
	created      []string
	removed      []string
	purges       int
	purgeRemoved int
	purgeErr     error
	purgeGate    chan struct{}
}

func (workspace *memoryWorkspace) Purge(ctx context.Context) (int, error) {
	if workspace.purgeGate != nil {
		<-workspace.purgeGate
	}

	if err := ctx.Err(); err != nil {
		return 0, err
	}

	workspace.mutex.Lock()
	defer workspace.mutex.Unlock()

	workspace.purges++

	return workspace.purgeRemoved, workspace.purgeErr
}

func (workspace *memoryWorkspace) purgeCount() int {
	workspace.mutex.Lock()
	defer workspace.mutex.Unlock()

	return workspace.purges
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

// clockWaiter is one pending After of the manual clock.
type clockWaiter struct {
	deadline time.Time
	fire     chan time.Time
}

// manualClock is a clock that only moves when the test advances it; advancing fires every
// After whose deadline has passed.
type manualClock struct {
	mutex   sync.Mutex
	now     time.Time
	waiters []clockWaiter
}

func (clock *manualClock) Now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()

	return clock.now
}

func (clock *manualClock) After(wait time.Duration) <-chan time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()

	// Buffered, so advancing never blocks on a receiver that has gone away.
	fire := make(chan time.Time, 1)
	clock.waiters = append(clock.waiters, clockWaiter{deadline: clock.now.Add(wait), fire: fire})

	return fire
}

func (clock *manualClock) advance(step time.Duration) {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()

	clock.now = clock.now.Add(step)

	pending := clock.waiters[:0]

	for _, waiter := range clock.waiters {
		if waiter.deadline.After(clock.now) {
			pending = append(pending, waiter)

			continue
		}

		waiter.fire <- clock.now
	}

	clock.waiters = pending
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

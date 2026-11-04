package watcher

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf16"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/sys/windows"
)

// recursiveNotifier is true: one ReadDirectoryChangesW call with bWatchSubtree covers the
// whole root (see the package documentation for why fsnotify is not used here). The single
// handle on the root is opened with FILE_SHARE_DELETE and blocks nothing below it.
const recursiveNotifier = true

const (
	// notifyBufferSize is the largest buffer SMB shares accept (fsnotify uses the same).
	notifyBufferSize = 64 * 1024
	notifyFilter     = windows.FILE_NOTIFY_CHANGE_FILE_NAME | windows.FILE_NOTIFY_CHANGE_DIR_NAME |
		windows.FILE_NOTIFY_CHANGE_SIZE | windows.FILE_NOTIFY_CHANGE_LAST_WRITE
	// Layout of FILE_NOTIFY_INFORMATION: three little-endian DWORDs, then the name in UTF-16.
	recordNextOffset       = 0
	recordActionOffset     = 4
	recordNameLengthOffset = 8
	recordHeaderSize       = 12
	utf16UnitSize          = 2
	// Arguments of the Win32 calls.
	manualReset    = 1
	initiallyUnset = 0
	stopEventIndex = windows.WAIT_OBJECT_0 + 1
	// notifierEventCount is the stop event and the read event.
	notifierEventCount = 2
	// stopPollMilliseconds bounds each wait, so the reader also notices quit when the stop
	// event could not be signalled.
	stopPollMilliseconds = 1000
)

// errStopped ends the read loop after close.
var errStopped = errors.New("notifier stopped")

// directoryNotifier watches a root recursively with overlapped ReadDirectoryChangesW calls
// on a reader goroutine. close signals stopEvent, which cancels the pending read.
//
// The kernel writes into buffer and overlapped while a read is pending, so both live in
// this heap-allocated struct: a goroutine stack may move, the heap does not. The reader
// keeps the struct reachable until the read has completed or was cancelled.
type directoryNotifier struct {
	root       string
	handle     windows.Handle
	stopEvent  windows.Handle
	ioEvent    windows.Handle
	overlapped *windows.Overlapped
	buffer     *[notifyBufferSize]byte
	eventsOut  chan fsnotify.Event
	errorsOut  chan error
	// quit unblocks a reader waiting to deliver, done is closed when the reader has ended.
	quit chan struct{}
	done chan struct{}
}

func openNotifier(root string) (notifier, error) {
	handle, err := openDirectory(root)
	if err != nil {
		return nil, err
	}

	events, err := createEvents(notifierEventCount)
	if err != nil {
		_ = windows.CloseHandle(handle)
		return nil, err
	}

	watcher := &directoryNotifier{
		root: root, handle: handle, stopEvent: events[0], ioEvent: events[1],
		overlapped: new(windows.Overlapped), buffer: new([notifyBufferSize]byte),
		eventsOut: make(chan fsnotify.Event), errorsOut: make(chan error),
		quit: make(chan struct{}), done: make(chan struct{}),
	}

	go watcher.read()

	return watcher, nil
}

func openDirectory(root string) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return windows.InvalidHandle, fmt.Errorf("encode root: %w", err)
	}

	handle, err := windows.CreateFile(name, windows.FILE_LIST_DIRECTORY,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return windows.InvalidHandle, fmt.Errorf("open root for change notifications: %w", err)
	}

	return handle, nil
}

// createEvents creates count manual-reset events, closing them all on failure.
func createEvents(count int) ([]windows.Handle, error) {
	events := make([]windows.Handle, 0, count)

	for range count {
		event, err := windows.CreateEvent(nil, manualReset, initiallyUnset, nil)
		if err != nil {
			_ = closeHandles(events...)
			return nil, fmt.Errorf("create event: %w", err)
		}

		events = append(events, event)
	}

	return events, nil
}

func closeHandles(handles ...windows.Handle) error {
	var errs []error

	for _, handle := range handles {
		if err := windows.CloseHandle(handle); err != nil {
			errs = append(errs, fmt.Errorf("close handle: %w", err))
		}
	}

	return errors.Join(errs...)
}

func (watcher *directoryNotifier) events() <-chan fsnotify.Event { return watcher.eventsOut }
func (watcher *directoryNotifier) failures() <-chan error        { return watcher.errorsOut }

// add and remove have nothing to do: the recursive notification covers every directory.
func (*directoryNotifier) add(string) error    { return nil }
func (*directoryNotifier) remove(string) error { return nil }

// close stops the reader and closes the handles only after it has ended, because a pending
// read still uses them. When the stop event cannot be signalled, the pending read is
// cancelled directly and the reader notices quit at its next bounded wait.
func (watcher *directoryNotifier) close() error {
	close(watcher.quit)

	stopErr := windows.SetEvent(watcher.stopEvent)
	if stopErr != nil {
		stopErr = fmt.Errorf("signal stop: %w", stopErr)
		_ = windows.CancelIoEx(watcher.handle, nil)
	}

	<-watcher.done

	return errors.Join(stopErr, closeHandles(watcher.handle, watcher.stopEvent, watcher.ioEvent))
}

// read delivers events until close or a failed read. It closes both channels when it ends,
// so a watch whose root handle broke ends instead of waiting for events that never come.
func (watcher *directoryNotifier) read() {
	defer close(watcher.done)
	defer close(watcher.errorsOut)
	defer close(watcher.eventsOut)

	for {
		count, err := watcher.readOnce()

		switch {
		case errors.Is(err, errStopped):
			return
		case err != nil:
			watcher.readFailed(err)
			return
		case count == 0:
			// The buffer overflowed: Windows dropped the changes and reports none.
			if !watcher.deliverFailure(fsnotify.ErrEventOverflow) {
				return
			}
		default:
			if !watcher.deliverRecords(watcher.buffer[:count]) {
				return
			}
		}
	}
}

// readOnce waits for one batch of changes in buffer and returns its size in bytes.
func (watcher *directoryNotifier) readOnce() (uint32, error) {
	if err := windows.ResetEvent(watcher.ioEvent); err != nil {
		return 0, fmt.Errorf("reset read event: %w", err)
	}

	*watcher.overlapped = windows.Overlapped{HEvent: watcher.ioEvent}

	err := windows.ReadDirectoryChanges(watcher.handle, &watcher.buffer[0], notifyBufferSize, true,
		notifyFilter, nil, watcher.overlapped, 0)
	if err != nil {
		return 0, fmt.Errorf("read directory changes: %w", err)
	}

	if !watcher.awaitRead() {
		watcher.cancelRead()
		return 0, errStopped
	}

	var count uint32

	err = windows.GetOverlappedResult(watcher.handle, watcher.overlapped, &count, false)
	if errors.Is(err, windows.ERROR_NOTIFY_ENUM_DIR) {
		return 0, nil
	}

	if err != nil {
		return 0, fmt.Errorf("read directory changes: %w", err)
	}

	return count, nil
}

// awaitRead waits until the pending read completes (true) or the notifier stops (false).
func (watcher *directoryNotifier) awaitRead() bool {
	handles := []windows.Handle{watcher.ioEvent, watcher.stopEvent}

	for {
		signalled, err := windows.WaitForMultipleObjects(handles, false, stopPollMilliseconds)

		switch {
		case err != nil || signalled == stopEventIndex:
			return false
		case signalled == windows.WAIT_OBJECT_0:
			return true
		}

		// Timeout: keep waiting unless close was called without a signalled stop event.
		select {
		case <-watcher.quit:
			return false
		default:
		}
	}
}

// cancelRead cancels the pending read and waits for it, because the kernel writes into
// buffer and overlapped until the read has completed.
func (watcher *directoryNotifier) cancelRead() {
	var ignored uint32

	if windows.CancelIoEx(watcher.handle, watcher.overlapped) == nil {
		_ = windows.GetOverlappedResult(watcher.handle, watcher.overlapped, &ignored, true)
	}
}

// readFailed reports a broken read. A root that no longer exists is reported as its removal,
// which ends the watch like the other platforms do.
func (watcher *directoryNotifier) readFailed(err error) {
	if _, statErr := os.Stat(watcher.root); statErr != nil {
		watcher.deliverEvent(fsnotify.Event{Name: watcher.root, Op: fsnotify.Remove})
		return
	}

	watcher.deliverFailure(err)
}

func (watcher *directoryNotifier) deliverRecords(records []byte) bool {
	for _, event := range parseRecords(watcher.root, records) {
		if !watcher.deliverEvent(event) {
			return false
		}
	}

	return true
}

func (watcher *directoryNotifier) deliverEvent(event fsnotify.Event) bool {
	select {
	case watcher.eventsOut <- event:
		return true
	case <-watcher.quit:
		return false
	}
}

func (watcher *directoryNotifier) deliverFailure(err error) bool {
	select {
	case watcher.errorsOut <- err:
		return true
	case <-watcher.quit:
		return false
	}
}

// parseRecords decodes a buffer of FILE_NOTIFY_INFORMATION records into events with
// absolute names. A truncated record ends the parse.
func parseRecords(root string, records []byte) []fsnotify.Event {
	var events []fsnotify.Event

	for offset := 0; offset+recordHeaderSize <= len(records); {
		next := int(binary.LittleEndian.Uint32(records[offset+recordNextOffset:]))
		action := binary.LittleEndian.Uint32(records[offset+recordActionOffset:])
		nameEnd := offset + recordHeaderSize + int(binary.LittleEndian.Uint32(records[offset+recordNameLengthOffset:]))

		if nameEnd > len(records) {
			break
		}

		if operation, ok := operationOf(action); ok {
			name := decodeUTF16(records[offset+recordHeaderSize : nameEnd])
			events = append(events, fsnotify.Event{Name: filepath.Join(root, name), Op: operation})
		}

		if next == 0 {
			break
		}

		offset += next
	}

	return events
}

// operationOf maps a FILE_ACTION code to the fsnotify operation; the new name of a rename
// arrives as a creation, like fsnotify reports it.
func operationOf(action uint32) (fsnotify.Op, bool) {
	switch action {
	case windows.FILE_ACTION_ADDED, windows.FILE_ACTION_RENAMED_NEW_NAME:
		return fsnotify.Create, true
	case windows.FILE_ACTION_REMOVED:
		return fsnotify.Remove, true
	case windows.FILE_ACTION_MODIFIED:
		return fsnotify.Write, true
	case windows.FILE_ACTION_RENAMED_OLD_NAME:
		return fsnotify.Rename, true
	default:
		return 0, false
	}
}

func decodeUTF16(encoded []byte) string {
	units := make([]uint16, len(encoded)/utf16UnitSize)
	for index := range units {
		units[index] = binary.LittleEndian.Uint16(encoded[index*utf16UnitSize:])
	}

	return string(utf16.Decode(units))
}

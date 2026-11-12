package stream

import (
	"log/slog"
	"net/http"
	"path/filepath"
	"slices"
	"sync"
	"testing"
)

// segmentRecorder collects the indexes reported through grant.requestSegment.
type segmentRecorder struct {
	mutex   sync.Mutex
	indexes []int
}

func (recorder *segmentRecorder) record(index int) {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()

	recorder.indexes = append(recorder.indexes, index)
}

func TestOnlySegmentRequestsReportTheirIndexForTheThrottle(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		fileName string
		want     []int
	}{
		"segment":        {fileName: "seg-00007.m4s", want: []int{7}},
		"first segment":  {fileName: "seg-00000.m4s", want: []int{0}},
		"large segment":  {fileName: "seg-12345.m4s", want: []int{12345}},
		"playlist":       {fileName: "index.m3u8"},
		"init":           {fileName: "init.mp4"},
		"invalid name":   {fileName: "seg-7.m4s"},
		"not served":     {fileName: "seg-0000a.m4s"},
		"wrong case":     {fileName: "SEG-00001.m4s"},
		"other file":     {fileName: "secret.txt"},
		"upper segments": {fileName: "seg-100000.m4s"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			directory := t.TempDir()
			for _, written := range []string{"seg-00007.m4s", "seg-00000.m4s", "seg-12345.m4s", "index.m3u8", "init.mp4"} {
				writeReleaseFile(t, filepath.Join(directory, written))
			}

			reported := &segmentRecorder{}
			template := hlsGrant(readyAtOnce)
			template.requestSegment = reported.record

			grants := &countingGrants{template: template}
			files := directoryFiles{directory: directory}
			handlers := &Handlers{
				access: grants.access, library: files, output: files,
				logger: slog.New(slog.DiscardHandler), readyTimeout: releaseReadyTimeout,
			}

			serveReleaseCase(t, handlers, releaseCase{route: hlsRoute, fileName: test.fileName})

			if !slices.Equal(reported.indexes, test.want) {
				t.Errorf("reported segments = %v, want %v", reported.indexes, test.want)
			}
		})
	}
}

func TestSegmentIndexParsesOnlyServedNames(t *testing.T) {
	t.Parallel()

	if index, ok := segmentIndex("seg-00042.m4s"); !ok || index != 42 {
		t.Errorf("segmentIndex(seg-00042.m4s) = %d, %v, want 42, true", index, ok)
	}

	if _, ok := segmentIndex(http.MethodGet); ok {
		t.Error("segmentIndex accepted a name that is no segment")
	}
}

package media_test

import (
	"errors"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

func TestFileValidateAcceptsProbedAndFailedFiles(t *testing.T) {
	t.Parallel()

	failed := probedFile()
	failed.Description = media.Description{}
	failed.ProbeError = media.ProbeErrorTimedOut

	for name, file := range map[string]media.File{"probed": probedFile(), "probe failed": failed} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if err := file.Validate(); err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestFileValidateRejectsBrokenValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*media.File)
	}{
		{name: "negative size", mutate: func(f *media.File) { f.Size = -1 }},
		{name: "empty relative path", mutate: func(f *media.File) { f.RelativePath = "" }},
		{name: "invalid description", mutate: func(f *media.File) { f.Description.Duration = -time.Second }},
		{
			name:   "probe error with streams",
			mutate: func(f *media.File) { f.ProbeError = media.ProbeErrorFailed },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			file := probedFile()
			test.mutate(&file)

			if err := file.Validate(); !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("Validate() error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestFileProbed(t *testing.T) {
	t.Parallel()

	file := probedFile()
	if !file.Probed() {
		t.Error("Probed() = false for a file without probe error, want true")
	}

	file.ProbeError = media.ProbeErrorFailed
	if file.Probed() {
		t.Error("Probed() = true for a file with probe error, want false")
	}
}

func TestProbeErrorCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code media.ProbeError
		want string
	}{
		{code: media.ProbeErrorNone, want: ""},
		{code: media.ProbeErrorFailed, want: "probe_failed"},
		{code: media.ProbeErrorTimedOut, want: "probe_timed_out"},
	}

	for _, test := range tests {
		if string(test.code) != test.want {
			t.Errorf("probe error code = %q, want %q", test.code, test.want)
		}
	}
}

func probedFile() media.File {
	return media.File{
		ID:           "4b0e7c9a-2f1d-4c3b-8e5a-6d7f8091a2b3",
		ItemID:       "9c8b7a6d-5e4f-4a3b-9c2d-1e0f9a8b7c6d",
		LibraryID:    "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d",
		RelativePath: "Films/HDR Demo.mkv",
		Size:         4_200_000,
		ModifiedAt:   time.Date(2026, time.October, 30, 12, 0, 0, 0, time.UTC),
		Description:  hdr10Description(),
	}
}

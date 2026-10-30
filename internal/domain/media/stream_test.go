package media_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

func TestParseStreamKindRoundTripsKnownKinds(t *testing.T) {
	t.Parallel()

	kinds := []media.StreamKind{
		media.StreamKindVideo, media.StreamKindAudio, media.StreamKindSubtitle, media.StreamKindOther,
	}
	want := []string{"video", "audio", "subtitle", "other"}

	for index, kind := range kinds {
		if string(kind) != want[index] {
			t.Errorf("stream kind %d = %q, want %q", index, kind, want[index])
		}

		got, err := media.ParseStreamKind(string(kind))
		if err != nil || got != kind {
			t.Errorf("ParseStreamKind(%q) = (%q, %v), want (%q, nil)", kind, got, err, kind)
		}
	}
}

func TestParseStreamKindRejectsOtherText(t *testing.T) {
	t.Parallel()

	for _, text := range []string{"", "Video", "data", "attachment", "subtitles"} {
		got, err := media.ParseStreamKind(text)
		if !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("ParseStreamKind(%q) error = %v, want ErrInvalid", text, err)
		}

		if got != "" {
			t.Errorf("ParseStreamKind(%q) = %q on error, want empty", text, got)
		}
	}
}

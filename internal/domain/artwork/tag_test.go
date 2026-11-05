package artwork_test

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain/artwork"
)

func baseTagInput() artwork.TagInput {
	return artwork.TagInput{
		Source:     artwork.SourceLocal,
		Location:   artwork.LocalLocation("Movie (1999)/poster.jpg"),
		Size:       123456,
		ModifiedAt: time.UnixMilli(1_700_000_000_123),
	}
}

func TestTagIsTheSHA256PrefixOfTheIdentity(t *testing.T) {
	t.Parallel()

	identity := "local\x00Movie (1999)/poster.jpg\x00-1\x00\x00123456\x001700000000123\x00" +
		strconv.Itoa(artwork.RenderVersion)
	digest := sha256.Sum256([]byte(identity))
	want := hex.EncodeToString(digest[:])[:artwork.TagLength]

	if got := artwork.Tag(baseTagInput()); got != want {
		t.Errorf("Tag() = %q, want %q", got, want)
	}
}

func TestTagChangesWithEveryIdentityPart(t *testing.T) {
	t.Parallel()

	base := artwork.Tag(baseTagInput())

	tests := []struct {
		name   string
		change func(input *artwork.TagInput)
	}{
		{name: "source", change: func(input *artwork.TagInput) { input.Source = artwork.SourceUpload }},
		{name: "relative path", change: func(input *artwork.TagInput) { input.Location.RelativePath = "Movie (1999)/folder.jpg" }},
		{name: "stream index", change: func(input *artwork.TagInput) { input.Location.StreamIndex = 2 }},
		{name: "data path", change: func(input *artwork.TagInput) { input.Location.DataPath = "artwork/x.jpg" }},
		{name: "size", change: func(input *artwork.TagInput) { input.Size++ }},
		{
			name:   "modification time in ms",
			change: func(input *artwork.TagInput) { input.ModifiedAt = input.ModifiedAt.Add(time.Millisecond) },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			input := baseTagInput()
			test.change(&input)

			if got := artwork.Tag(input); got == base {
				t.Errorf("Tag() after changing %s = %q, want a different tag", test.name, got)
			}
		})
	}
}

func TestTagIgnoresSubMillisecondTimeAndZone(t *testing.T) {
	t.Parallel()

	base := artwork.Tag(baseTagInput())

	input := baseTagInput()
	input.ModifiedAt = input.ModifiedAt.Add(999 * time.Microsecond).In(time.FixedZone("UTC+2", 2*60*60))

	if got := artwork.Tag(input); got != base {
		t.Errorf("Tag() with sub-millisecond change = %q, want %q", got, base)
	}
}

func TestTagHasFixedLengthLowercaseHex(t *testing.T) {
	t.Parallel()

	tag := artwork.Tag(artwork.TagInput{})
	if len(tag) != artwork.TagLength {
		t.Fatalf("len(Tag()) = %d, want %d", len(tag), artwork.TagLength)
	}

	for _, character := range tag {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			t.Fatalf("Tag() = %q, want lowercase hex", tag)
		}
	}
}

func TestCandidateTagMatchesTag(t *testing.T) {
	t.Parallel()

	input := baseTagInput()
	candidate := artwork.Candidate{
		Kind:       artwork.KindPoster,
		Source:     input.Source,
		Priority:   3,
		Location:   input.Location,
		Size:       input.Size,
		ModifiedAt: input.ModifiedAt,
	}

	if got, want := candidate.Tag(), artwork.Tag(input); got != want {
		t.Errorf("Candidate.Tag() = %q, want %q", got, want)
	}
}

func TestPhotoTagIsTheTagOfTheLocalFile(t *testing.T) {
	t.Parallel()

	input := baseTagInput()

	got := artwork.PhotoTag(input.Location.RelativePath, input.Size, input.ModifiedAt)
	if want := artwork.Tag(input); got != want {
		t.Errorf("PhotoTag = %q, want %q", got, want)
	}

	later := input.ModifiedAt.Add(time.Second)
	if changed := artwork.PhotoTag(input.Location.RelativePath, input.Size, later); changed == got {
		t.Error("PhotoTag did not change with the modification time")
	}
}

package restapi

import (
	"math"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

func TestStreamKindResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind media.StreamKind
		want apigen.MediaStreamKind
	}{
		{kind: media.StreamKindVideo, want: apigen.MediaStreamKindVideo},
		{kind: media.StreamKindAudio, want: apigen.MediaStreamKindAudio},
		{kind: media.StreamKindSubtitle, want: apigen.MediaStreamKindSubtitle},
		{kind: media.StreamKindOther, want: apigen.MediaStreamKindOther},
		{kind: "data", want: apigen.MediaStreamKindOther},
	}

	for _, test := range tests {
		if got := streamKindResponse(test.kind); got != test.want {
			t.Errorf("streamKindResponse(%q) = %q, want %q", test.kind, got, test.want)
		}
	}
}

func TestHDRResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		format media.HDRFormat
		want   *apigen.HdrFormat
	}{
		{format: media.HDRFormatHDR10, want: new(apigen.HdrFormatHdr10)},
		{format: media.HDRFormatHLG, want: new(apigen.HdrFormatHlg)},
		{format: media.HDRFormatNone},
		{format: "dolby_vision"},
	}

	for _, test := range tests {
		got := hdrResponse(test.format)
		if (got == nil) != (test.want == nil) || (got != nil && *got != *test.want) {
			t.Errorf("hdrResponse(%q) = %v, want %v", test.format, got, test.want)
		}
	}
}

func TestOptionalInt32(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value int
		want  *int32
	}{
		{value: math.MinInt32 - 1},
		{value: -1},
		{value: 0},
		{value: 1, want: new(int32(1))},
		{value: math.MaxInt32, want: new(int32(math.MaxInt32))},
		{value: math.MaxInt32 + 1},
	}

	for _, test := range tests {
		got := optionalInt32(test.value)
		if (got == nil) != (test.want == nil) || (got != nil && *got != *test.want) {
			t.Errorf("optionalInt32(%d) = %v, want %v", test.value, got, test.want)
		}
	}
}

// TestStreamIndexOutOfRangeFailsTheItem: clients select audio streams by index, so an index
// the contract cannot carry fails the response instead of being clamped.
func TestStreamIndexOutOfRangeFailsTheItem(t *testing.T) {
	t.Parallel()

	for _, index := range []int{-1, math.MaxInt32 + 1} {
		detail := applibrary.ItemDetail{
			Item: library.Item{ID: "8a9b0c1d-2e3f-4a5b-8c6d-7e8f9a0b1c2d", LibraryID: "3f6c1d2a-9b8e-4f70-8a1c-2d3e4f5a6b7c"},
			Files: []media.File{{
				ID:          "1c2d3e4f-5a6b-4c7d-8e9f-0a1b2c3d4e5f",
				Description: media.Description{Streams: []media.Stream{{Index: index, Kind: media.StreamKindAudio}}},
			}},
		}

		if _, err := itemDetailResponse(detail); err == nil {
			t.Errorf("itemDetailResponse with stream index %d succeeded, want an error", index)
		}
	}
}

func TestItemSummaryWithMalformedItemIDFails(t *testing.T) {
	t.Parallel()

	summary := applibrary.ItemSummary{Item: library.Item{ID: "item-1", LibraryID: "3f6c1d2a-9b8e-4f70-8a1c-2d3e4f5a6b7c"}}

	if _, err := itemSummaryResponse(summary); err == nil {
		t.Error("itemSummaryResponse with a malformed item id succeeded, want an error")
	}
}

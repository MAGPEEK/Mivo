package main

import (
	"errors"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/corpustest"
	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

func TestReferenceProfileMatchesTheADRTable(t *testing.T) {
	t.Parallel()

	chromium, err := referenceProfile(corpustest.ChromiumDesktop)
	if err != nil {
		t.Fatalf("referenceProfile: %v", err)
	}

	if chromium.Name != corpustest.ChromiumDesktop || chromium.Hls.Delivery != apigen.HlsDeliveryMse || len(chromium.DirectPlay) != 2 {
		t.Errorf("chromium = %+v, want MSE HLS and mp4 + webm direct play", chromium)
	}

	if chromium.CodecLimits == nil || len(*chromium.CodecLimits) != 3 {
		t.Fatalf("chromium codec limits = %v, want h264, vp9, av1", chromium.CodecLimits)
	}

	h264 := (*chromium.CodecLimits)[0]
	if h264.Codec != "h264" || h264.MaxBitDepth == nil || *h264.MaxBitDepth != 8 || h264.Profiles == nil ||
		!slices.Contains(*h264.Profiles, "High") {
		t.Errorf("h264 limit = %+v, want 8 bit with the High profile", h264)
	}

	if vp9 := (*chromium.CodecLimits)[1]; vp9.Profiles != nil {
		t.Errorf("vp9 limit has profiles %v, want them omitted", *vp9.Profiles)
	}

	firefox, err := referenceProfile(corpustest.FirefoxDesktop)
	if err != nil || len(firefox.DirectPlay) != 3 || firefox.DirectPlay[2].Container != "ogg" {
		t.Errorf("firefox = %+v, %v; want ogg as the third direct-play container", firefox, err)
	}
}

func TestReferenceProfileRejectsUnknownNames(t *testing.T) {
	t.Parallel()

	if _, err := referenceProfile("safari-ios"); !errors.Is(err, errUnknownProfile) {
		t.Errorf("referenceProfile = %v, want errUnknownProfile", err)
	}
}

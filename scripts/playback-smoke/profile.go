package main

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/magpeek/mivo/internal/adapters/corpustest"
	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

var errUnknownProfile = errors.New("unknown reference profile")

// referenceProfile returns the reference profile of ADR 0025 with the given name in the
// shape of the API contract. The profiles come from playbacktest, the source the corpus
// integration test and the expectation table use, so the smoke test cannot drift from them.
func referenceProfile(name string) (apigen.ClientProfile, error) {
	profiles := map[string]func() playback.ClientProfile{
		corpustest.ChromiumDesktop: playbacktest.ChromiumDesktop,
		corpustest.FirefoxDesktop:  playbacktest.FirefoxDesktop,
	}

	build, found := profiles[name]
	if !found {
		return apigen.ClientProfile{}, fmt.Errorf("%w %q, want one of %s",
			errUnknownProfile, name, strings.Join(slices.Sorted(maps.Keys(profiles)), ", "))
	}

	return contractProfile(build()), nil
}

// contractProfile is the inverse of the REST adapter's profile conversion: a bit depth of
// 0 (unlimited) and an empty profile list are omitted, as a client would.
func contractProfile(profile playback.ClientProfile) apigen.ClientProfile {
	converted := apigen.ClientProfile{
		Name:       profile.Name,
		DirectPlay: make([]apigen.DirectPlayProfile, 0, len(profile.DirectPlay)),
		Hls: apigen.HlsProfile{
			Delivery:    apigen.HlsDelivery(profile.HLS.Delivery),
			VideoCodecs: slices.Clone(profile.HLS.VideoCodecs),
			AudioCodecs: slices.Clone(profile.HLS.AudioCodecs),
		},
	}

	for _, entry := range profile.DirectPlay {
		converted.DirectPlay = append(converted.DirectPlay, apigen.DirectPlayProfile{
			Container:   string(entry.Container),
			VideoCodecs: slices.Clone(entry.VideoCodecs),
			AudioCodecs: slices.Clone(entry.AudioCodecs),
		})
	}

	if len(profile.CodecLimits) == 0 {
		return converted
	}

	limits := make([]apigen.CodecLimit, 0, len(profile.CodecLimits))

	for _, limit := range profile.CodecLimits {
		entry := apigen.CodecLimit{Codec: limit.Codec}

		if limit.MaxBitDepth > 0 {
			depth := int32(limit.MaxBitDepth) //nolint:gosec // reference bit depths are 8 or 10
			entry.MaxBitDepth = &depth
		}

		if len(limit.Profiles) > 0 {
			profiles := slices.Clone(limit.Profiles)
			entry.Profiles = &profiles
		}

		limits = append(limits, entry)
	}

	converted.CodecLimits = &limits

	return converted
}

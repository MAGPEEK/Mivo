package restapi

import (
	"fmt"
	"math"
	"time"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// maxStartPositionSeconds is the largest start position a time.Duration holds (about 292
// years). A larger float has no defined conversion; it is at or beyond any file's duration
// anyway, which the contract answers with 400 "invalid".
const maxStartPositionSeconds = float64(math.MaxInt64 / int64(time.Second))

// minCodecLimitBitDepth is the contract's minimum of CodecLimit.maxBitDepth. The domain reads
// 0 as "unlimited", which a client expresses by omitting the field, so an explicit 0 is a
// broken client.
const minCodecLimitBitDepth = 1

// newStartRequest turns the request body into the app's request and validates what the
// generated binding does not: the schema bounds the decoder ignores and the client profile.
// The profile is validated here, before the service runs, so a broken or hostile profile
// never reaches the decision engine or the start log. Errors wrap domain.ErrInvalid.
func newStartRequest(body *apigen.PlaybackRequest) (appplayback.StartRequest, error) {
	if body == nil {
		return appplayback.StartRequest{}, fmt.Errorf("missing request body: %w", domain.ErrInvalid)
	}

	// A missing fileId decodes to the nil UUID, which no catalogued file has.
	if body.FileID == uuid.Nil() {
		return appplayback.StartRequest{}, fmt.Errorf("missing file id: %w", domain.ErrInvalid)
	}

	startPosition, err := newStartPosition(body.StartPositionSeconds)
	if err != nil {
		return appplayback.StartRequest{}, err
	}

	audioStreamIndex, err := newAudioStreamIndex(body.AudioStreamIndex)
	if err != nil {
		return appplayback.StartRequest{}, err
	}

	profile, err := newClientProfile(body.Profile)
	if err != nil {
		return appplayback.StartRequest{}, err
	}

	request := appplayback.StartRequest{
		FileID:           body.FileID.String(),
		Profile:          profile,
		StartPosition:    startPosition,
		AudioStreamIndex: audioStreamIndex,
	}

	if body.ReplacesSessionID != nil {
		request.ReplacesSessionID = body.ReplacesSessionID.String()
	}

	return request, nil
}

// newStartPosition converts seconds to a duration; nil starts at the beginning.
func newStartPosition(seconds *float64) (time.Duration, error) {
	if seconds == nil {
		return 0, nil
	}

	if *seconds < 0 || *seconds > maxStartPositionSeconds {
		return 0, fmt.Errorf("start position %g s outside 0..%g s: %w", *seconds, maxStartPositionSeconds, domain.ErrInvalid)
	}

	return time.Duration(math.Round(*seconds * float64(time.Second))), nil
}

// newAudioStreamIndex converts the optional index; nil plays the default audio stream.
func newAudioStreamIndex(index *int32) (*int, error) {
	if index == nil {
		return nil, nil
	}

	if *index < 0 {
		return nil, fmt.Errorf("negative audio stream index %d: %w", *index, domain.ErrInvalid)
	}

	converted := int(*index)

	return &converted, nil
}

// newClientProfile converts the profile and validates it with the domain's bounds (ADR 0023).
func newClientProfile(profile apigen.ClientProfile) (playback.ClientProfile, error) {
	converted := playback.ClientProfile{
		Name:       profile.Name,
		DirectPlay: make([]playback.DirectPlayProfile, 0, len(profile.DirectPlay)),
		HLS: playback.HLSProfile{
			Delivery:    playback.HLSDelivery(profile.Hls.Delivery),
			VideoCodecs: profile.Hls.VideoCodecs,
			AudioCodecs: profile.Hls.AudioCodecs,
		},
	}

	for _, entry := range profile.DirectPlay {
		converted.DirectPlay = append(converted.DirectPlay, playback.DirectPlayProfile{
			Container:   media.DirectPlayContainer(entry.Container),
			VideoCodecs: entry.VideoCodecs,
			AudioCodecs: entry.AudioCodecs,
		})
	}

	limits, err := newCodecLimits(profile.CodecLimits)
	if err != nil {
		return playback.ClientProfile{}, err
	}

	converted.CodecLimits = limits

	if err := converted.Validate(); err != nil {
		return playback.ClientProfile{}, fmt.Errorf("client profile: %w", err)
	}

	return converted, nil
}

// newCodecLimits converts the optional limits; nil means none.
func newCodecLimits(limits *[]apigen.CodecLimit) ([]playback.CodecLimit, error) {
	if limits == nil {
		return nil, nil
	}

	converted := make([]playback.CodecLimit, 0, len(*limits))

	for index, limit := range *limits {
		entry := playback.CodecLimit{Codec: limit.Codec}

		if limit.MaxBitDepth != nil {
			if *limit.MaxBitDepth < minCodecLimitBitDepth {
				return nil, fmt.Errorf("codec limit %d: max bit depth %d below %d: %w",
					index, *limit.MaxBitDepth, minCodecLimitBitDepth, domain.ErrInvalid)
			}

			entry.MaxBitDepth = int(*limit.MaxBitDepth)
		}

		if limit.Profiles != nil {
			entry.Profiles = *limit.Profiles
		}

		converted = append(converted, entry)
	}

	return converted, nil
}

package restapi

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

// pathSeparators are cut off in front of a file name. Stored paths use "/", but a Linux file
// name may contain "\", which a Windows client would read as a directory.
const pathSeparators = `/\`

func itemPageResponse(page applibrary.ItemPage) (apigen.ItemPage, error) {
	items := make([]apigen.ItemSummary, 0, len(page.Items))

	for _, summary := range page.Items {
		item, err := itemSummaryResponse(summary)
		if err != nil {
			return apigen.ItemPage{}, err
		}

		items = append(items, item)
	}

	return apigen.ItemPage{Items: items, NextCursor: optionalString(page.NextCursor)}, nil
}

func itemSummaryResponse(summary applibrary.ItemSummary) (apigen.ItemSummary, error) {
	id, err := parseStoredID("item", summary.Item.ID)
	if err != nil {
		return apigen.ItemSummary{}, err
	}

	libraryID, err := parseStoredID("library", summary.Item.LibraryID)
	if err != nil {
		return apigen.ItemSummary{}, fmt.Errorf("item %s: %w", summary.Item.ID, err)
	}

	return apigen.ItemSummary{
		ID:        id,
		LibraryID: libraryID,
		Title:     summary.Item.Title,
		Media:     mediaSummaryResponse(summary.PrimaryFile),
	}, nil
}

// mediaSummaryResponse describes the primary file for list views. A file whose probe failed
// has a zero description, so only "available": false remains.
func mediaSummaryResponse(file media.File) *apigen.MediaSummary {
	description := file.Description
	summary := &apigen.MediaSummary{
		Available:       file.Probed(),
		Container:       containerResponse(description.Container),
		DurationSeconds: durationSeconds(description.Duration),
	}

	if video, found := description.PrimaryVideo(); found {
		summary.VideoCodec = optionalString(video.Codec)
		summary.Width = optionalInt32(video.Width)
		summary.Height = optionalInt32(video.Height)
		summary.BitDepth = optionalInt32(video.BitDepth)
		summary.Hdr = hdrResponse(video.HDR)
	}

	if audio, found := description.DefaultAudio(); found {
		summary.AudioCodec = optionalString(audio.Codec)
	}

	return summary
}

func itemDetailResponse(detail applibrary.ItemDetail) (apigen.ItemDetail, error) {
	id, err := parseStoredID("item", detail.Item.ID)
	if err != nil {
		return apigen.ItemDetail{}, err
	}

	libraryID, err := parseStoredID("library", detail.Item.LibraryID)
	if err != nil {
		return apigen.ItemDetail{}, fmt.Errorf("item %s: %w", detail.Item.ID, err)
	}

	files := make([]apigen.MediaFile, 0, len(detail.Files))

	for _, file := range detail.Files {
		response, err := mediaFileResponse(file)
		if err != nil {
			return apigen.ItemDetail{}, fmt.Errorf("item %s: %w", detail.Item.ID, err)
		}

		files = append(files, response)
	}

	return apigen.ItemDetail{ID: id, LibraryID: libraryID, Title: detail.Item.Title, Files: files}, nil
}

// mediaFileResponse sends the file name only, never the relative path (ADR 0022).
func mediaFileResponse(file media.File) (apigen.MediaFile, error) {
	id, err := parseStoredID("file", file.ID)
	if err != nil {
		return apigen.MediaFile{}, err
	}

	streams := make([]apigen.MediaStream, 0, len(file.Description.Streams))

	for _, stream := range file.Description.Streams {
		response, err := mediaStreamResponse(stream)
		if err != nil {
			return apigen.MediaFile{}, fmt.Errorf("file %s: %w", file.ID, err)
		}

		streams = append(streams, response)
	}

	return apigen.MediaFile{
		ID:              id,
		FileName:        fileName(file.RelativePath),
		SizeBytes:       file.Size,
		ModifiedAt:      apiTimestamp(file.ModifiedAt),
		Available:       file.Probed(),
		ErrorCode:       optionalString(string(file.ProbeError)),
		Container:       containerResponse(file.Description.Container),
		DurationSeconds: durationSeconds(file.Description.Duration),
		BitRate:         optionalInt64(file.Description.BitRate),
		Streams:         streams,
	}, nil
}

// mediaStreamResponse fails for an index the contract cannot carry: clients select audio
// streams by it, so it must not be clamped or dropped.
func mediaStreamResponse(stream media.Stream) (apigen.MediaStream, error) {
	if stream.Index < 0 || stream.Index > math.MaxInt32 {
		return apigen.MediaStream{}, fmt.Errorf("stream index %d out of range", stream.Index)
	}

	return apigen.MediaStream{
		Index:           int32(stream.Index),
		Kind:            streamKindResponse(stream.Kind),
		Codec:           optionalString(stream.Codec),
		Profile:         optionalString(stream.Profile),
		Level:           optionalInt32(stream.Level),
		BitDepth:        optionalInt32(stream.BitDepth),
		Width:           optionalInt32(stream.Width),
		Height:          optionalInt32(stream.Height),
		FrameRate:       frameRateResponse(stream.FrameRate),
		Hdr:             hdrResponse(stream.HDR),
		Channels:        optionalInt32(stream.Channels),
		ChannelLayout:   optionalString(stream.ChannelLayout),
		SampleRate:      optionalInt32(stream.SampleRate),
		Language:        optionalString(stream.Language),
		Title:           optionalString(stream.Title),
		Default:         stream.Default,
		Forced:          stream.Forced,
		AttachedPicture: stream.AttachedPicture,
		BitRate:         optionalInt64(stream.BitRate),
	}, nil
}

// streamKindResponse sends a kind this adapter does not know as "other", which the contract
// defines for every stream that is neither video, audio nor subtitle.
func streamKindResponse(kind media.StreamKind) apigen.MediaStreamKind {
	switch kind {
	case media.StreamKindVideo:
		return apigen.MediaStreamKindVideo
	case media.StreamKindAudio:
		return apigen.MediaStreamKindAudio
	case media.StreamKindSubtitle:
		return apigen.MediaStreamKindSubtitle
	case media.StreamKindOther:
		return apigen.MediaStreamKindOther
	default:
		return apigen.MediaStreamKindOther
	}
}

// hdrResponse omits SDR and any format the contract does not list.
func hdrResponse(format media.HDRFormat) *apigen.HdrFormat {
	var response apigen.HdrFormat

	switch format {
	case media.HDRFormatHDR10:
		response = apigen.HdrFormatHdr10
	case media.HDRFormatHLG:
		response = apigen.HdrFormatHlg
	case media.HDRFormatNone:
		return nil
	default:
		return nil
	}

	return &response
}

// containerResponse omits an unknown container, as the contract says.
func containerResponse(container media.Container) *string {
	if container == media.ContainerUnknown {
		return nil
	}

	return optionalString(string(container))
}

// durationSeconds sends a known duration in seconds with millisecond precision.
func durationSeconds(duration time.Duration) *float64 {
	if duration <= 0 {
		return nil
	}

	seconds := float64(duration.Round(time.Millisecond)) / float64(time.Second)

	return &seconds
}

func frameRateResponse(rate media.FrameRate) *float64 {
	if !rate.IsKnown() {
		return nil
	}

	framesPerSecond := rate.RoundedFramesPerSecond()

	return &framesPerSecond
}

// fileName returns the last element of a root-relative path.
func fileName(relativePath string) string {
	return relativePath[strings.LastIndexAny(relativePath, pathSeparators)+1:]
}

// optionalInt32 omits 0, which the domain uses for "not reported", and values the contract's
// int32 cannot carry; they are no real width, level or sample rate.
func optionalInt32(value int) *int32 {
	if value <= 0 || value > math.MaxInt32 {
		return nil
	}

	converted := int32(value)

	return &converted
}

// optionalInt64 omits 0, which the domain uses for "not reported".
func optionalInt64(value int64) *int64 {
	if value <= 0 {
		return nil
	}

	return &value
}

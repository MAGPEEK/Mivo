package restapi

import (
	"fmt"
	"strings"

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

	response := apigen.ItemPage{Items: items, NextCursor: optionalString(page.NextCursor)}
	if page.Counted {
		total := page.TotalCount
		response.TotalCount = &total
	}

	return response, nil
}

// itemSummaryResponse maps a listing entry. Containers own no file and so send no media
// summary, nor does a still photo; absent numbers, counts and artwork are omitted.
func itemSummaryResponse(summary applibrary.ItemSummary) (apigen.ItemSummary, error) {
	item := summary.Item

	identity, err := itemIdentity(item)
	if err != nil {
		return apigen.ItemSummary{}, err
	}

	response := apigen.ItemSummary{
		ID:             identity.id,
		LibraryID:      identity.libraryID,
		ParentID:       identity.parentID,
		Kind:           identity.kind,
		Title:          item.Title,
		Year:           optionalNumber(item.Year),
		Number:         optionalNumber(item.Number),
		NumberEnd:      optionalNumber(item.NumberEnd),
		AbsoluteNumber: optionalNumber(item.AbsoluteNumber),
		DiscNumber:     discNumberResponse(item.DiscNumber),
		ExtraType:      extraTypeResponse(item.ExtraType),
		ChildCount:     optionalInt32(summary.ChildCount),
		LeafCount:      optionalInt32(summary.LeafCount),
	}

	response.Media = itemMediaSummary(summary)

	if err := applyPhotoSummary(&response, summary); err != nil {
		return apigen.ItemSummary{}, err
	}

	poster, err := imageRefResponse(summary.Poster)
	if err != nil {
		return apigen.ItemSummary{}, fmt.Errorf("item %s: poster: %w", item.ID, err)
	}

	if showsSquareArtwork(item.Kind) {
		response.Square = poster
	} else {
		response.Poster = poster
	}

	response.Thumb, err = imageRefResponse(summary.Thumb)
	if err != nil {
		return apigen.ItemSummary{}, fmt.Errorf("item %s: thumb: %w", item.ID, err)
	}

	return response, nil
}

// itemMediaSummary describes the primary file of a listed item (see showsMediaSummary); an
// audiobook reports the summed length of its parts.
func itemMediaSummary(summary applibrary.ItemSummary) *apigen.MediaSummary {
	if !showsMediaSummary(summary) {
		return nil
	}

	media := mediaSummaryResponse(summary.PrimaryFile)
	if summary.PartsDuration > 0 {
		media.DurationSeconds = optionalDurationSeconds(summary.PartsDuration)
	}

	return media
}

// mediaSummaryResponse describes the primary file for list views. A file whose probe failed
// has a zero description, so only "available": false remains.
func mediaSummaryResponse(file media.File) *apigen.MediaSummary {
	description := file.Description
	summary := &apigen.MediaSummary{
		Available:       file.Probed(),
		Container:       containerResponse(description.Container),
		DurationSeconds: optionalDurationSeconds(description.Duration),
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

// itemFileResponse maps a file with its place among the item's files.
func itemFileResponse(itemFile applibrary.ItemFile) (apigen.MediaFile, error) {
	response, err := mediaFileResponse(itemFile.File)
	if err != nil {
		return apigen.MediaFile{}, err
	}

	response.VersionLabel = optionalString(itemFile.VersionLabel)
	response.PartNumber = optionalNumber(itemFile.PartNumber)

	return response, nil
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

	// A disc folder is named after the folder holding BDMV or VIDEO_TS, not "VIDEO_TS".
	return apigen.MediaFile{
		ID:              id,
		FileName:        fileName(file.Layout.DiscRoot(file.RelativePath)),
		SizeBytes:       file.Size,
		ModifiedAt:      apiTimestamp(file.ModifiedAt),
		Available:       file.Probed(),
		ErrorCode:       optionalString(string(file.ProbeError)),
		Container:       containerResponse(file.Description.Container),
		DurationSeconds: optionalDurationSeconds(file.Description.Duration),
		BitRate:         optionalInt64(file.Description.BitRate),
		Streams:         streams,
	}, nil
}

// mediaStreamResponse fails for an index the contract cannot carry (streamIndex).
func mediaStreamResponse(stream media.Stream) (apigen.MediaStream, error) {
	index, err := streamIndex(stream.Index)
	if err != nil {
		return apigen.MediaStream{}, err
	}

	return apigen.MediaStream{
		Index:           index,
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

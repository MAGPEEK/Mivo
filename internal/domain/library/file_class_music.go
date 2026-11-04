package library

import (
	"path"
	"strings"
)

// FileClassAudio is an audio file of a music or audiobooks library. ClassifyFile does not
// return it, because the same extension (".mp4", ".ogg") may be video elsewhere; scans use
// ClassifyFileIn with the library kind.
const FileClassAudio FileClass = "audio"

// IsAudioFileName reports whether the name has an audio extension (ignoring case):
// flac m4a m4b alac mp3 aac opus ogg wav aiff ape wv dsf dff. M4B is the audiobook
// container; it is shared with music because the extension set is one list.
func IsAudioFileName(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".flac", ".m4a", ".m4b", ".alac", ".mp3", ".aac", ".opus", ".ogg",
		".wav", ".aiff", ".ape", ".wv", ".dsf", ".dff":
		return true
	default:
		return false
	}
}

// isAudioKind reports whether libraries of the kind hold audio files instead of video.
func isAudioKind(kind Kind) bool {
	return kind == KindMusic || kind == KindAudiobooks
}

// AllowedIn reports whether a library of this kind may contain files of this class. Audio
// is valid for music and audiobooks only, photos for photos libraries only, video for every
// kind but the audio ones; NFO and image sidecars are valid everywhere. It agrees with
// ClassifyFileIn.
func (class FileClass) AllowedIn(kind Kind) bool {
	switch class {
	case FileClassAudio:
		return isAudioKind(kind)
	case FileClassPhoto:
		return kind == KindPhotos
	case FileClassVideo:
		return !isAudioKind(kind)
	case FileClassNFO, FileClassImage:
		return true
	default:
		return false
	}
}

// ClassifyFileIn classifies a file for a library of the given kind: audio files in music and
// audiobooks libraries, otherwise the result of ClassifyFile. Audio libraries hold no video
// files; NFO and image sidecars stay valid.
func ClassifyFileIn(kind Kind, name string) (class FileClass, ok bool) {
	if FileClassAudio.AllowedIn(kind) && IsAudioFileName(name) {
		return FileClassAudio, true
	}

	if kind == KindPhotos {
		if class, ok = classifyPhotoFile(name); ok {
			return class, ok
		}
	}

	class, ok = ClassifyFile(name)
	if ok && !class.AllowedIn(kind) {
		return "", false
	}

	return class, ok
}

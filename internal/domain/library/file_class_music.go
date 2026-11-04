package library

import (
	"path"
	"strings"
)

// FileClassAudio is an audio file of a music library. ClassifyFile does not return it,
// because the same extension (".mp4", ".ogg") may be video elsewhere; scans use
// ClassifyFileIn with the library kind.
const FileClassAudio FileClass = "audio"

// IsAudioFileName reports whether the name has an audio extension (ignoring case):
// flac m4a alac mp3 aac opus ogg wav aiff ape wv dsf dff.
func IsAudioFileName(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".flac", ".m4a", ".alac", ".mp3", ".aac", ".opus", ".ogg",
		".wav", ".aiff", ".ape", ".wv", ".dsf", ".dff":
		return true
	default:
		return false
	}
}

// AllowedIn reports whether a library of this kind may contain files of this class. Audio is
// valid for music only here; the audiobook kind joins when it is introduced.
func (class FileClass) AllowedIn(kind Kind) bool {
	if class == FileClassAudio {
		return kind == KindMusic
	}

	if class == FileClassPhoto {
		return kind == KindPhotos
	}

	return class.IsValid()
}

// ClassifyFileIn classifies a file for a library of the given kind: audio files in music
// libraries, images and videos in photos libraries (images are photos there), otherwise the result of ClassifyFile. A music library holds no video files; NFO
// and image sidecars stay valid.
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
	if ok && class == FileClassVideo && kind == KindMusic {
		return "", false
	}

	return class, ok
}

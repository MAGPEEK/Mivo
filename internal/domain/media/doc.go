// Package media describes media files technically: container, duration, bit rate and the
// streams with their codec, colour, HDR and track flags, as ffprobe reports them after the
// adapter normalised its output (ADR 0022, ADR 0023).
//
// The split between media and library is deliberate. media answers "what is inside this
// file" and is read by the scanner, the playback decision engine and the API's technical
// chips. internal/domain/library answers "how is the collection organised" (libraries,
// items, file-name and path policies). Neither package imports the other, so the technical
// model can be reused for sources that are not library files (DLNA, uploads) later on.
//
// The package never parses ffprobe JSON (that is adapters/ffmpeg) and never touches the file
// system. Codec names are ffprobe codec_name strings; containers use Mivo's short names.
//
// Dolby Vision is not detected in milestone 2: a Dolby Vision profile 8 stream is reported
// by its HDR10 or HLG base layer, and a profile 5 stream is reported as SDR. Detection
// arrives with tone mapping in milestone 4.
package media

// Package naming turns library-relative paths into placements (ADR 0027 §4): which movie a
// file belongs to, its title, year, version label and part number, which extra of which
// movie it is, or that it is a sample to ignore. It never touches the file system; the scan
// supplies the path (Unicode NFC, "/"-separated) and the number of videos in the file's
// directory. Titles keep their original letters; only match keys are case-folded.
package naming

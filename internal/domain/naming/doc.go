// Package naming turns library-relative paths into placements (ADR 0027 §4): which movie or
// which show, season and episode a file belongs to, its title, year, version label and part
// number, which extra it is, or that it is a sample to ignore. It never touches the file
// system; the scan supplies the path (Unicode NFC, "/"-separated) and the number of videos in
// the file's directory. Titles keep their original letters; only match keys are case-folded.
//
// Shows (ADR 0027 §4 "Shows"): the first directory is the show, season folders and episode
// markers set the season, and an episode number is either explicit (S01E02, 1x02, E02,
// Episode 2) or a bare 1–4 digit number after the title ("Show - 105"). A bare number is the
// episode number and the absolute number and is never split into season and episode; years,
// resolutions, codec tags, checksums and "v2" revisions are not numbers. Keys are the
// season number, the episode number ("5" or "5-6") or "title:<name>" for unnumbered
// episodes, so renaming an episode title keeps the item. A file name "<marker> - <Title> -
// <Label>" has the episode title before and the version label after the second dash; two
// files with the same episode key are versions of one episode. A top-level folder or root
// file whose name has no letters or digits yields domain.ErrInvalid.
package naming

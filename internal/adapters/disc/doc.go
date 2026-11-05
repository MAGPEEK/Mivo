// Package disc selects the main title of a Blu-ray (BDMV) or DVD (VIDEO_TS) disc in pure Go,
// without libbluray or libdvdnav (ADR 0029).
//
// Blu-ray: every PLAYLIST/*.mpls is parsed and the longest playlist wins that neither repeats
// a clip (menu and decoy loops) nor references a missing STREAM clip. Multi-angle play items
// use angle 1. In/out times of a play item only contribute its duration; the whole clip file
// is played. DVD: the title set (VTS_nn) with the largest total size of its title VOBs
// (VTS_nn_1.VOB …) wins; its duration comes from the longest program chain in VTS_nn_0.IFO
// (or the .BUP copy) when that parses.
//
// Disc data is hostile input: parsers bound every offset and count by the bytes read, files are
// read with size limits and a per-disc byte budget, directory listings are capped, and clip
// paths are built only from validated names (five-digit clip ids, fixed VOB patterns) found by
// listing the disc itself, so a crafted playlist cannot point outside the disc. Folder discs are
// read through os.Root, images through the iso package.
//
// Known limits (ADR 0029): obfuscated Blu-rays whose decoy playlists are as long as the feature
// and seamless-branching discs may select the wrong title. The BDMV folder is looked up only at
// the tree root or as its direct child, so AVCHD nesting (PRIVATE/AVCHD/BDMV) is found only when
// the walker reports that BDMV folder itself; inside ISO images it is not found.
package disc

package disc

import (
	"fmt"
	"io/fs"
	"time"

	"github.com/magpeek/mivo/internal/domain/media"
)

// DVD names: VTS_nn_k.VOB (nn = title set 01..99, k = 0 menu, 1..9 title parts) and
// VTS_nn_0.IFO with its backup VTS_nn_0.BUP, compared lower-cased.
const (
	videoTSFolderName = "video_ts"
	maxTitleSets      = 99
	maxTitleParts     = 9

	titleSetNameLength = len("vts_01_1.vob")
	titleSetPrefix     = "vts_"
	titleSetNumberAt   = 4
	titleSetPartAt     = 7
	titleSetExtAt      = 8
	vobExtension       = ".vob"
	ifoExtension       = ".ifo"
	bupExtension       = ".bup"

	// decimalBase combines two decimal digits (set numbers, BCD bytes).
	decimalBase = 10
)

// titleSet collects the tree paths of one VTS; vobs is indexed by part number (0 unused).
type titleSet struct {
	vobs     [maxTitleParts + 1]string
	ifo, bup string
}

// selectVideoTS picks the title set with the largest total size of its title VOBs. Ties keep
// the lowest set number; a set with a gap in its part numbers is skipped (missing clip).
func selectVideoTS(files *fileTree) ([]titleClip, error) {
	base, err := files.discFolder(videoTSFolderName)
	if err != nil {
		return nil, fmt.Errorf("select DVD title: %w", err)
	}
	entries, err := files.list(base)
	if err != nil {
		return nil, fmt.Errorf("select DVD title: %w", err)
	}
	sets := groupTitleSets(base, entries)

	var best *titleSet
	var bestPaths []string
	var bestSizes []int64
	var bestTotal int64
	for number := 1; number <= maxTitleSets; number++ {
		paths, sizes, err := files.titleSetParts(&sets[number])
		if err != nil {
			return nil, fmt.Errorf("select DVD title: %w", err)
		}
		if total := sum(sizes); total > bestTotal {
			best, bestPaths, bestSizes, bestTotal = &sets[number], paths, sizes, total
		}
	}
	if best == nil {
		return nil, fmt.Errorf("select DVD title: no title set with VOBs: %w", media.ErrDiscNoTitle)
	}

	duration, err := files.titleSetDuration(best)
	if err != nil {
		return nil, fmt.Errorf("select DVD title: %w", err)
	}
	shares := splitDuration(duration, bestSizes)
	clips := make([]titleClip, len(bestPaths))
	for index, clipPath := range bestPaths {
		clips[index] = titleClip{path: clipPath, duration: shares[index]}
	}

	return clips, nil
}

// groupTitleSets sorts the VOB/IFO/BUP entries of the VIDEO_TS folder into their title sets.
func groupTitleSets(base string, entries []fs.DirEntry) *[maxTitleSets + 1]titleSet {
	var sets [maxTitleSets + 1]titleSet
	for _, entry := range entries {
		number, part, extension, ok := parseTitleSetName(lowerASCII(entry.Name()))
		if !ok || entry.IsDir() {
			continue
		}
		set, entryPath := &sets[number], joinPath(base, entry.Name())
		switch {
		case extension == vobExtension && part > 0 && set.vobs[part] == "":
			set.vobs[part] = entryPath
		case extension == ifoExtension && part == 0 && set.ifo == "":
			set.ifo = entryPath
		case extension == bupExtension && part == 0 && set.bup == "":
			set.bup = entryPath
		}
	}

	return &sets
}

// parseTitleSetName splits "vts_nn_k.ext" into set number (1..99), part (0..9) and extension.
func parseTitleSetName(name string) (int, int, string, bool) {
	if len(name) != titleSetNameLength || name[:titleSetNumberAt] != titleSetPrefix ||
		name[titleSetPartAt-1] != '_' || name[titleSetExtAt] != '.' {
		return 0, 0, "", false
	}
	tens, ones, part := name[titleSetNumberAt], name[titleSetNumberAt+1], name[titleSetPartAt]
	if !isDigit(tens) || !isDigit(ones) || !isDigit(part) {
		return 0, 0, "", false
	}
	number := int(tens-'0')*decimalBase + int(ones-'0')
	if number == 0 {
		return 0, 0, "", false
	}

	return number, int(part - '0'), name[titleSetExtAt:], true
}

func isDigit(char byte) bool { return '0' <= char && char <= '9' }

// titleSetParts returns the paths and sizes of parts 1..n of a set. A set with a gap, an
// empty or unreadable part yields nothing; only cancellation is an error.
func (files *fileTree) titleSetParts(set *titleSet) ([]string, []int64, error) {
	var paths []string
	var sizes []int64
	for part := 1; part <= maxTitleParts; part++ {
		if set.vobs[part] == "" {
			if hasPartsFrom(set, part) {
				return nil, nil, nil
			}
			break
		}
		size, err := files.size(set.vobs[part])
		if err != nil && !isSkippable(err) {
			return nil, nil, err
		}
		if err != nil || size <= 0 {
			return nil, nil, nil
		}
		paths, sizes = append(paths, set.vobs[part]), append(sizes, size)
	}

	return paths, sizes, nil
}

func hasPartsFrom(set *titleSet, first int) bool {
	for part := first; part <= maxTitleParts; part++ {
		if set.vobs[part] != "" {
			return true
		}
	}

	return false
}

// titleSetDuration reads the playback time from the IFO, else its BUP copy; zero when
// neither parses (the duration is then unknown, FFmpeg's probe reports it later).
func (files *fileTree) titleSetDuration(set *titleSet) (time.Duration, error) {
	for _, name := range []string{set.ifo, set.bup} {
		if name == "" {
			continue
		}
		data, err := files.read(name, maxIFOSize)
		if err != nil && !isSkippable(err) {
			return 0, err
		}
		if err != nil {
			continue
		}
		if duration, err := parseTitleSetDuration(data); err == nil {
			return duration, nil
		}
	}

	return 0, nil
}

func sum(values []int64) int64 {
	var total int64
	for _, value := range values {
		total += value
	}

	return total
}

package photo

import (
	"cmp"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/domain/library"
)

// Entry is one photo of a folder for sorting: its file name and capture time.
type Entry struct {
	Name        string
	CaptureTime time.Time
}

// Sort orders entries by capture time, then by file name (case-insensitive, then exact so
// the order is total). It sorts in place and is stable.
func Sort(entries []Entry) {
	slices.SortStableFunc(entries, func(a, b Entry) int {
		return cmp.Or(a.CaptureTime.Compare(b.CaptureTime), compareNames(a.Name, b.Name))
	})
}

func compareNames(a, b string) int {
	return cmp.Or(strings.Compare(strings.ToLower(a), strings.ToLower(b)), strings.Compare(a, b))
}

// Group is one photo item: the primary file and the alternate files that belong to it,
// for example the RAW file next to its JPEG.
type Group struct {
	Primary    string
	Alternates []string
}

// GroupFiles pairs the files of one folder (base names) into photo items. A RAW file joins
// the non-RAW image with the same stem (case-insensitive); the best of several candidates is
// web, then HEIC, then TIFF, ties by name. Everything else, including a RAW file without a
// partner, videos and other images with an equal stem, is its own item. Files that are
// neither image nor video are skipped. The result is ordered by primary file name.
func GroupFiles(names []string) []Group {
	partners := bestPartners(names)
	groups := map[string]*Group{}

	for _, name := range names {
		if !isPhotoFile(name) {
			continue
		}

		primary := name
		if partner, paired := partners[stemKey(name)]; paired && isRAW(name) {
			primary = partner
		}

		group := groups[primary]
		if group == nil {
			group = &Group{Primary: primary}
			groups[primary] = group
		}

		if primary != name {
			group.Alternates = append(group.Alternates, name)
		}
	}

	return collect(groups)
}

// bestPartners maps each stem to the non-RAW image that a RAW file of that stem joins.
func bestPartners(names []string) map[string]string {
	partners := map[string]string{}

	for _, name := range names {
		class, ok := library.ClassifyImage(name)
		if !ok || class == library.ImageClassRAW {
			continue
		}

		stem := stemKey(name)
		if best, exists := partners[stem]; !exists || better(name, best) {
			partners[stem] = name
		}
	}

	return partners
}

// collect turns the group map into a slice ordered by primary name, alternates sorted.
func collect(groups map[string]*Group) []Group {
	order := make([]string, 0, len(groups))
	for primary := range groups {
		order = append(order, primary)
	}

	slices.SortFunc(order, compareNames)

	result := make([]Group, 0, len(order))

	for _, primary := range order {
		group := *groups[primary]
		slices.Sort(group.Alternates)
		result = append(result, group)
	}

	return result
}

func isPhotoFile(name string) bool {
	if _, ok := library.ClassifyImage(name); ok {
		return true
	}

	return library.IsVideoFileName(name)
}

func isRAW(name string) bool {
	class, ok := library.ClassifyImage(name)

	return ok && class == library.ImageClassRAW
}

func stemKey(name string) string {
	return strings.ToLower(strings.TrimSuffix(name, path.Ext(name)))
}

// better reports whether candidate beats current as the primary of a pair.
func better(candidate, current string) bool {
	candidateRank, currentRank := rank(candidate), rank(current)
	if candidateRank != currentRank {
		return candidateRank < currentRank
	}

	return candidate < current
}

// rank orders the non-RAW classes by how directly a browser can show them (lower is better).
func rank(name string) int {
	class, _ := library.ClassifyImage(name)

	return slices.Index(partnerPreference(), class)
}

// partnerPreference lists the classes a RAW file can join, best first.
func partnerPreference() []library.ImageClass {
	return []library.ImageClass{library.ImageClassWeb, library.ImageClassHEIC, library.ImageClassTIFF}
}

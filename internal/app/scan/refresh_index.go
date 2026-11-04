package scan

import (
	"path"
	"slices"
	"strings"

	"github.com/magpeek/mivo/internal/domain/library"
)

// refreshItem is an item of a refresh request with the files that describe it.
type refreshItem struct {
	id    string
	level library.Level
	// own are the files the item holds itself (movie, episode); below are all files of the
	// item and of its descendants (a show's episodes).
	own   []RefreshFile
	below []RefreshFile
}

// itemIndex derives the items of a refresh request from its files.
type itemIndex struct {
	items map[string]*refreshItem
	// moviesPerDirectory counts, per directory, the movies that have a file there; a movie
	// alone in a folder is a folder movie.
	moviesPerDirectory map[string]map[string]bool
}

func newItemIndex(files []RefreshFile) *itemIndex {
	sorted := slices.Clone(files)
	slices.SortFunc(sorted, func(first, second RefreshFile) int {
		return strings.Compare(first.State.RelativePath, second.State.RelativePath)
	})

	index := &itemIndex{items: map[string]*refreshItem{}, moviesPerDirectory: map[string]map[string]bool{}}

	for _, file := range sorted {
		if len(file.ItemIDs) != len(file.Placement.Levels) || len(file.ItemIDs) == 0 {
			continue
		}

		index.add(file)
	}

	return index
}

func (index *itemIndex) add(file RefreshFile) {
	last := len(file.ItemIDs) - 1

	for position, id := range file.ItemIDs {
		item := index.items[id]
		if item == nil {
			item = &refreshItem{id: id, level: file.Placement.Levels[position]}
			index.items[id] = item
		}

		item.below = append(item.below, file)

		if position != last {
			continue
		}

		item.own = append(item.own, file)

		if item.level.Kind == library.ItemKindMovie {
			index.noteMovie(path.Dir(file.State.RelativePath), id)
		}
	}
}

func (index *itemIndex) noteMovie(directory, itemID string) {
	if index.moviesPerDirectory[directory] == nil {
		index.moviesPerDirectory[directory] = map[string]bool{}
	}

	index.moviesPerDirectory[directory][itemID] = true
}

// isFolderMovie reports whether the movie lives alone in a folder below the root.
func (index *itemIndex) isFolderMovie(directory string) bool {
	return directory != "." && len(index.moviesPerDirectory[directory]) == 1
}

// order lists the IDs to look at: the requested ones present in the index first, then the
// others sorted, which are only refreshed when they have no metadata yet.
func (index *itemIndex) order(requested []string) (ids []string, isRequested map[string]bool) {
	isRequested = make(map[string]bool, len(requested))

	for _, id := range requested {
		if index.items[id] != nil && !isRequested[id] {
			isRequested[id] = true

			ids = append(ids, id)
		}
	}

	var others []string

	for id := range index.items {
		if !isRequested[id] {
			others = append(others, id)
		}
	}

	slices.Sort(others)

	return append(ids, others...), isRequested
}

// showDirectory is the top folder of the show's files; empty for files in the root.
func (item *refreshItem) showDirectory() string {
	for _, file := range item.below {
		if first, _, nested := strings.Cut(file.State.RelativePath, relativePathSeparator); nested {
			return first
		}
	}

	return ""
}

// seasonDirectory is the folder of the season's episodes when it is not the show folder.
func (item *refreshItem) seasonDirectory() string {
	showDirectory := item.showDirectory()

	for _, file := range item.below {
		directory := path.Dir(file.State.RelativePath)
		if directory != "." && directory != showDirectory {
			return directory
		}
	}

	return ""
}

// baseName is the file name without its extension.
func baseName(relativePath string) string {
	name := path.Base(relativePath)

	return strings.TrimSuffix(name, path.Ext(name))
}

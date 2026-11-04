package iso

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"testing"
)

const walkBudget = 2000

// walker visits a tree reading the start of every file, but stops after a fixed number of
// operations, as every caller of a hostile image must (the tree may be an exponential DAG).
type walker struct {
	img    *Image
	budget int
}

func (w *walker) spend() bool {
	w.budget--
	return w.budget >= 0
}

func (w *walker) walk(dir string, depth int) {
	if depth > maxPathDepth+1 || !w.spend() {
		return
	}
	entries, err := w.img.ReadDir(dir)
	if err != nil {
		return
	}
	for _, de := range entries {
		name := dir + "/" + de.Name()
		if dir == "." {
			name = de.Name()
		}
		if de.IsDir() {
			w.walk(name, depth+1)
			continue
		}
		if !w.spend() {
			return
		}
		if handle, err := w.img.Open(name); err == nil {
			_, _ = io.CopyN(io.Discard, handle, 1<<16)
		}
		_, _ = w.img.Extents(name)
	}
}

func openAndWalk(data []byte) {
	img, err := Open(context.Background(), bytes.NewReader(data), int64(len(data)))
	if err == nil {
		(&walker{img: img, budget: walkBudget}).walk(".", 0)
	}
}

// graphVolume is a synthetic directory graph: next maps a directory identity to its subdirectories.
type graphVolume struct {
	next  func(id int64) []int64
	lists int
}

func (g *graphVolume) root() entry { return entry{name: ".", dir: true, resolved: true} }

func (g *graphVolume) list(_ context.Context, dir entry) ([]entry, error) {
	g.lists++
	var out []entry
	for i, target := range g.next(dir.id.first) {
		out = append(out, entry{name: fmt.Sprintf("d%d", i), dir: true, resolved: true, id: nodeID{first: target}})
	}
	return out, nil
}

func (g *graphVolume) resolve(context.Context, *entry) error { return nil }

func graphImage(g *graphVolume) *Image {
	return &Image{vol: g, cache: newListCache(), ctx: context.Background()}
}

func TestDirectoryCyclesAreRejected(t *testing.T) {
	tests := map[string]func(id int64) []int64{
		"all children point at the root": func(int64) []int64 { return []int64{0, 0, 0} },
		"grandchild points at parent": func(id int64) []int64 {
			if id == 0 {
				return []int64{1, 2}
			}
			return []int64{id}
		},
	}
	for name, next := range tests {
		g := &graphVolume{next: next}
		img := graphImage(g)
		if _, err := img.Open("d0/d0"); !errors.Is(err, ErrInvalidImage) {
			t.Errorf("%s: Open = %v, want ErrInvalidImage", name, err)
		}
		visited := 0
		_ = fs.WalkDir(img, ".", func(string, fs.DirEntry, error) error {
			visited++
			return nil
		})
		if visited > 10 || g.lists > 4 {
			t.Errorf("%s: visited %d entries, %d listings", name, visited, g.lists)
		}
	}
}

func TestDeepPathIsRejected(t *testing.T) {
	g := &graphVolume{next: func(id int64) []int64 { return []int64{id + 1} }}
	img := graphImage(g)
	path := "d0"
	for range maxPathDepth {
		path += "/d0"
	}
	if _, err := img.Open(path); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("Open(deep) = %v, want ErrInvalid", err)
	}
}

// A diamond ladder has 2^depth paths but only 2*depth directories: with the listing cache the
// parse work stays proportional to the directories, not the paths.
func TestDiamondDagListsEachDirectoryOnce(t *testing.T) {
	const levels = 100
	g := &graphVolume{next: func(id int64) []int64 {
		level := id/2 + 1
		if id == 0 {
			level = 1
		}
		if level >= levels {
			return nil
		}
		return []int64{2 * level, 2*level + 1}
	}}
	w := &walker{img: graphImage(g), budget: walkBudget}
	w.walk(".", 0)
	if w.budget >= 0 {
		t.Fatalf("walk finished with %d operations left; the DAG should exhaust the budget", w.budget)
	}
	if g.lists > 2*maxPathDepth+2 {
		t.Errorf("%d directory parses for a ladder of %d levels", g.lists, maxPathDepth)
	}
}

func TestListingCacheBoundsAndHits(t *testing.T) {
	g := &graphVolume{next: func(int64) []int64 { return nil }}
	img := graphImage(g)
	for range 5 {
		if _, err := img.ReadDir("."); err != nil {
			t.Fatal(err)
		}
	}
	if g.lists != 1 {
		t.Errorf("root parsed %d times, want 1", g.lists)
	}
	cache := newListCache()
	for id := range int64(maxCachedDirs * 2) {
		cache.put(nodeID{first: id}, []entry{{name: "x"}})
	}
	if len(cache.dirs) > maxCachedDirs {
		t.Errorf("cache holds %d directories, cap %d", len(cache.dirs), maxCachedDirs)
	}
}

func TestISO9660KeepsHighestVersion(t *testing.T) {
	listing := isoListing{versions: map[string]int{}, index: map[string]int{}}
	for _, version := range []int{1, 3, 2} {
		item := entry{name: "A", size: int64(version)}
		if err := listing.add(item, version, false); err != nil {
			t.Fatal(err)
		}
	}
	if len(listing.entries) != 1 || listing.entries[0].size != 3 {
		t.Errorf("entries = %+v, want only version 3", listing.entries)
	}
}

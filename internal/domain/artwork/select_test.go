package artwork_test

import (
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain/artwork"
)

func localCandidate(relativePath string, priority int) artwork.Candidate {
	return artwork.Candidate{
		Kind:     artwork.KindPoster,
		Source:   artwork.SourceLocal,
		Priority: priority,
		Location: artwork.LocalLocation(relativePath),
	}
}

func embeddedCandidate(relativePath string, streamIndex int) artwork.Candidate {
	return artwork.Candidate{
		Kind:     artwork.KindPoster,
		Source:   artwork.SourceEmbedded,
		Location: artwork.EmbeddedLocation(relativePath, streamIndex),
	}
}

func dataCandidate(source artwork.SourceKind, dataPath string) artwork.Candidate {
	return artwork.Candidate{Kind: artwork.KindPoster, Source: source, Location: artwork.DataLocation(dataPath)}
}

func TestSelect(t *testing.T) {
	t.Parallel()

	folderJPG := localCandidate("Movie/folder.jpg", 1)
	posterJPG := localCandidate("Movie/poster.jpg", 0)
	posterPNG := localCandidate("Movie/poster.png", 0)
	cover := embeddedCandidate("Movie/Movie.mkv", 3)
	coverEarlierStream := embeddedCandidate("Movie/Movie.mkv", 2)
	coverOtherFile := embeddedCandidate("Movie/Movie - 4K.mkv", 5)
	provider := dataCandidate(artwork.SourceProvider, "artwork/b.jpg")
	upload := dataCandidate(artwork.SourceUpload, "artwork/a.jpg")
	unknown := dataCandidate("future", "artwork/0.jpg")

	tests := []struct {
		name       string
		candidates []artwork.Candidate
		locked     bool
		current    *artwork.Candidate
		want       artwork.Candidate
		selected   bool
	}{
		{name: "no candidates", selected: false},
		{name: "no candidates keeps nothing even with a current", current: &posterJPG, selected: false},
		{name: "local by priority", candidates: []artwork.Candidate{folderJPG, posterJPG}, want: posterJPG, selected: true},
		{name: "local before embedded", candidates: []artwork.Candidate{cover, folderJPG}, want: folderJPG, selected: true},
		{name: "embedded without local", candidates: []artwork.Candidate{cover}, want: cover, selected: true},
		{name: "tie broken by relative path", candidates: []artwork.Candidate{posterPNG, posterJPG}, want: posterJPG, selected: true},
		{name: "embedded tie by file", candidates: []artwork.Candidate{cover, coverOtherFile}, want: coverOtherFile, selected: true},
		{name: "embedded tie by stream", candidates: []artwork.Candidate{cover, coverEarlierStream}, want: coverEarlierStream, selected: true},
		{name: "embedded before provider", candidates: []artwork.Candidate{provider, cover}, want: cover, selected: true},
		{name: "provider before upload", candidates: []artwork.Candidate{upload, provider}, want: provider, selected: true},
		{name: "unknown source last", candidates: []artwork.Candidate{unknown, upload}, want: upload, selected: true},
		{
			name: "unlocked ignores current", candidates: []artwork.Candidate{folderJPG, posterJPG},
			current: &folderJPG, want: posterJPG, selected: true,
		},
		{
			name: "locked keeps current", candidates: []artwork.Candidate{folderJPG, posterJPG},
			locked: true, current: &folderJPG, want: folderJPG, selected: true,
		},
		{
			name: "locked keeps a current that is no longer a candidate", candidates: []artwork.Candidate{posterJPG},
			locked: true, current: &upload, want: upload, selected: true,
		},
		{name: "locked without current selects nothing", candidates: []artwork.Candidate{posterJPG}, locked: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, selected := artwork.Select(test.candidates, test.locked, test.current)
			if got != test.want || selected != test.selected {
				t.Errorf("Select() = %+v, %t, want %+v, %t", got, selected, test.want, test.selected)
			}
		})
	}
}

func TestSelectDoesNotDependOnCandidateOrder(t *testing.T) {
	t.Parallel()

	candidates := []artwork.Candidate{
		embeddedCandidate("Movie/Movie.mkv", 1),
		localCandidate("Movie/poster.png", 0),
		localCandidate("Movie/folder.jpg", 1),
		localCandidate("Movie/poster.jpg", 0),
	}
	want := localCandidate("Movie/poster.jpg", 0)

	for range len(candidates) {
		if got, _ := artwork.Select(candidates, false, nil); got != want {
			t.Fatalf("Select(%v) = %+v, want %+v", candidates, got, want)
		}

		candidates = append(candidates[1:], candidates[0])
	}

	slices.Reverse(candidates)

	if got, _ := artwork.Select(candidates, false, nil); got != want {
		t.Errorf("Select(reversed) = %+v, want %+v", got, want)
	}
}

package musicbrainz

// The types below mirror only the MusicBrainz JSON fields the adapter maps; everything else
// in an answer is ignored by encoding/json.

type releaseGroupSearchResponse struct {
	ReleaseGroups []releaseGroup `json:"release-groups"`
}

type artistSearchResponse struct {
	Artists []artist `json:"artists"`
}

// releaseGroup is an album across all its editions, as searched and as looked up.
type releaseGroup struct {
	ID               string  `json:"id"`
	Score            float64 `json:"score"`
	Title            string  `json:"title"`
	FirstReleaseDate string  `json:"first-release-date"`
	Genres           []vote  `json:"genres"`
	Tags             []vote  `json:"tags"`
}

// release is one edition of an album.
type release struct {
	ID              string         `json:"id"`
	Title           string         `json:"title"`
	Date            string         `json:"date"`
	ReleaseGroup    *releaseGroup  `json:"release-group"`
	LabelInfo       []labelInfo    `json:"label-info"`
	Genres          []vote         `json:"genres"`
	Tags            []vote         `json:"tags"`
	CoverArtArchive coverArtStatus `json:"cover-art-archive"`
}

type labelInfo struct {
	Label *label `json:"label"`
}

type label struct {
	Name string `json:"name"`
}

type coverArtStatus struct {
	Front bool `json:"front"`
}

type artist struct {
	ID       string  `json:"id"`
	Score    float64 `json:"score"`
	Name     string  `json:"name"`
	SortName string  `json:"sort-name"`
	Area     *area   `json:"area"`
	Genres   []vote  `json:"genres"`
	Tags     []vote  `json:"tags"`
}

type area struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// vote is a genre or folksonomy tag with the number of users who applied it.
type vote struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

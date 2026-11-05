package tmdb

// The types below mirror the parts of the TMDB v3 answers the adapter reads. They never leave
// the package (ADR 0031 §6).

type searchResponse struct {
	Results []searchResult `json:"results"`
}

// searchResult is a movie (title, release_date) or show (name, first_air_date) hit.
type searchResult struct {
	ID            int64   `json:"id"`
	Title         string  `json:"title"`
	OriginalTitle string  `json:"original_title"`
	ReleaseDate   string  `json:"release_date"`
	Name          string  `json:"name"`
	OriginalName  string  `json:"original_name"`
	FirstAirDate  string  `json:"first_air_date"`
	Popularity    float64 `json:"popularity"`
}

type findResponse struct {
	MovieResults []searchResult `json:"movie_results"`
	TVResults    []searchResult `json:"tv_results"`
}

type namedEntry struct {
	Name string `json:"name"`
}

type castMember struct {
	Name      string `json:"name"`
	Character string `json:"character"`
	Order     int    `json:"order"`
}

type crewMember struct {
	Name       string `json:"name"`
	Job        string `json:"job"`
	Department string `json:"department"`
}

type credits struct {
	Cast []castMember `json:"cast"`
	Crew []crewMember `json:"crew"`
}

type externalIDs struct {
	IMDbID string `json:"imdb_id"`
	TVDBID int64  `json:"tvdb_id"`
}

type image struct {
	FilePath string `json:"file_path"`
}

type images struct {
	Posters   []image `json:"posters"`
	Backdrops []image `json:"backdrops"`
	Logos     []image `json:"logos"`
}

type releaseDates struct {
	Results []struct {
		Region       string `json:"iso_3166_1"`
		ReleaseDates []struct {
			Certification string `json:"certification"`
		} `json:"release_dates"`
	} `json:"results"`
}

type contentRatings struct {
	Results []struct {
		Region string `json:"iso_3166_1"`
		Rating string `json:"rating"`
	} `json:"results"`
}

// detailsBase holds the fields movies and shows share.
type detailsBase struct {
	ID                  int64        `json:"id"`
	Overview            string       `json:"overview"`
	Tagline             string       `json:"tagline"`
	Genres              []namedEntry `json:"genres"`
	ProductionCountries []namedEntry `json:"production_countries"`
	PosterPath          string       `json:"poster_path"`
	BackdropPath        string       `json:"backdrop_path"`
	Credits             credits      `json:"credits"`
	ExternalIDs         externalIDs  `json:"external_ids"`
	Images              images       `json:"images"`
}

type movieDetails struct {
	detailsBase

	Title               string       `json:"title"`
	OriginalTitle       string       `json:"original_title"`
	ReleaseDate         string       `json:"release_date"`
	IMDbID              string       `json:"imdb_id"`
	ProductionCompanies []namedEntry `json:"production_companies"`
	BelongsToCollection *namedEntry  `json:"belongs_to_collection"`
	ReleaseDates        releaseDates `json:"release_dates"`
}

type showDetails struct {
	detailsBase

	Name           string         `json:"name"`
	OriginalName   string         `json:"original_name"`
	FirstAirDate   string         `json:"first_air_date"`
	Networks       []namedEntry   `json:"networks"`
	ContentRatings contentRatings `json:"content_ratings"`
}

type seasonDetails struct {
	Name         string           `json:"name"`
	Overview     string           `json:"overview"`
	AirDate      string           `json:"air_date"`
	SeasonNumber int              `json:"season_number"`
	PosterPath   string           `json:"poster_path"`
	Episodes     []episodeDetails `json:"episodes"`
}

type episodeDetails struct {
	EpisodeNumber int          `json:"episode_number"`
	Name          string       `json:"name"`
	Overview      string       `json:"overview"`
	AirDate       string       `json:"air_date"`
	StillPath     string       `json:"still_path"`
	Crew          []crewMember `json:"crew"`
	GuestStars    []castMember `json:"guest_stars"`
}

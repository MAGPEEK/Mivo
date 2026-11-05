package enrich

import (
	"encoding/json"
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// errProviderDisabled answers a call to a provider whose key was refused; it wraps
// domain.ErrUnavailable so the queue backs off instead of retrying at once.
var errProviderDisabled = fmt.Errorf("provider disabled until the settings change: %w", domain.ErrUnavailable)

// The cache payload is provider-neutral JSON of these shapes. metadata.Date has no exported
// fields, so a record carries its release date as ISO text; the outer ReleaseDate field is
// shallower than the embedded one and therefore wins in encoding/json.
type (
	cachedRecord struct {
		metadata.Record
		ReleaseDate string `json:"ReleaseDate,omitempty"`
	}
	cachedDetails struct {
		Record  cachedRecord
		Artwork []ArtworkRef `json:",omitempty"`
	}
	cachedEpisode struct {
		Number  int
		Details cachedDetails
	}
	cachedSeason struct {
		Season   cachedDetails
		Episodes []cachedEpisode `json:",omitempty"`
	}
)

func candidatesCodec() codec[[]match.Candidate] {
	return codec[[]match.Candidate]{
		encode: func(candidates []match.Candidate) ([]byte, error) { return json.Marshal(candidates) },
		decode: func(payload []byte) ([]match.Candidate, error) {
			var candidates []match.Candidate
			if err := json.Unmarshal(payload, &candidates); err != nil {
				return nil, fmt.Errorf("decode candidates: %w", err)
			}

			return candidates, nil
		},
	}
}

func detailsCodec() codec[Details] {
	return codec[Details]{
		encode: func(details Details) ([]byte, error) { return json.Marshal(toCachedDetails(details)) },
		decode: func(payload []byte) (Details, error) {
			var cached cachedDetails
			if err := json.Unmarshal(payload, &cached); err != nil {
				return Details{}, fmt.Errorf("decode details: %w", err)
			}

			return cached.details()
		},
	}
}

func seasonCodec() codec[SeasonDetails] {
	return codec[SeasonDetails]{
		encode: func(season SeasonDetails) ([]byte, error) { return json.Marshal(toCachedSeason(season)) },
		decode: func(payload []byte) (SeasonDetails, error) {
			var cached cachedSeason
			if err := json.Unmarshal(payload, &cached); err != nil {
				return SeasonDetails{}, fmt.Errorf("decode season: %w", err)
			}

			return cached.season()
		},
	}
}

func toCachedRecord(record metadata.Record) cachedRecord {
	cached := cachedRecord{Record: record}
	cached.Record.ReleaseDate = metadata.Date{}

	if !record.ReleaseDate.IsZero() {
		cached.ReleaseDate = record.ReleaseDate.String()
	}

	return cached
}

func (cached cachedRecord) record() (metadata.Record, error) {
	record := cached.Record
	if cached.ReleaseDate == "" {
		return record, nil
	}

	date, err := metadata.ParseDate(cached.ReleaseDate)
	if err != nil {
		return metadata.Record{}, fmt.Errorf("decode release date: %w", err)
	}

	record.ReleaseDate = date

	return record, nil
}

func toCachedDetails(details Details) cachedDetails {
	return cachedDetails{Record: toCachedRecord(details.Record), Artwork: details.Artwork}
}

func (cached cachedDetails) details() (Details, error) {
	record, err := cached.Record.record()
	if err != nil {
		return Details{}, err
	}

	return Details{Record: record, Artwork: cached.Artwork}, nil
}

func toCachedSeason(season SeasonDetails) cachedSeason {
	cached := cachedSeason{Season: toCachedDetails(season.Details)}
	for _, episode := range season.Episodes {
		cached.Episodes = append(cached.Episodes, cachedEpisode{Number: episode.Number, Details: toCachedDetails(episode.Details)})
	}

	return cached
}

func (cached cachedSeason) season() (SeasonDetails, error) {
	details, err := cached.Season.details()
	if err != nil {
		return SeasonDetails{}, err
	}

	season := SeasonDetails{Details: details}

	for _, episode := range cached.Episodes {
		episodeDetails, err := episode.Details.details()
		if err != nil {
			return SeasonDetails{}, fmt.Errorf("episode %d: %w", episode.Number, err)
		}

		season.Episodes = append(season.Episodes, EpisodeDetails{Number: episode.Number, Details: episodeDetails})
	}

	return season, nil
}

package tmdb

import (
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/domain/metadata"
)

const (
	// maxCast bounds the actors taken from a credit list; TMDB lists hundreds for big films.
	maxCast = 20

	jobDirector       = "Director"
	jobProducer       = "Producer"
	departmentWriting = "Writing"
)

// movieRecord maps movie details to a record.
func movieRecord(details movieDetails, lang language) metadata.Record {
	record := baseRecord(details.detailsBase)
	record.Title = details.Title
	record.OriginalTitle = details.OriginalTitle
	record.ReleaseDate, record.Year = parseDate(details.ReleaseDate)
	record.Studios = names(details.ProductionCompanies)
	record.ContentRating = movieCertification(details.ReleaseDates, lang.region)

	if details.BelongsToCollection != nil && details.BelongsToCollection.Name != "" {
		record.Collections = []string{details.BelongsToCollection.Name}
	}

	external := details.ExternalIDs
	if external.IMDbID == "" {
		external.IMDbID = details.IMDbID
	}

	record.ProviderIDs = providerIDs(details.ID, external)

	return record
}

// showRecord maps show details to a record; networks count as studios as in Kodi NFOs.
func showRecord(details showDetails, lang language) metadata.Record {
	record := baseRecord(details.detailsBase)
	record.Title = details.Name
	record.OriginalTitle = details.OriginalName
	record.ReleaseDate, record.Year = parseDate(details.FirstAirDate)
	record.Studios = names(details.Networks)
	record.ContentRating = showRating(details.ContentRatings, lang.region)
	record.ProviderIDs = providerIDs(details.ID, details.ExternalIDs)

	return record
}

func baseRecord(details detailsBase) metadata.Record {
	return metadata.Record{
		Summary:   details.Overview,
		Tagline:   details.Tagline,
		Genres:    names(details.Genres),
		Countries: names(details.ProductionCountries),
		People:    append(castCredits(details.Credits.Cast, metadata.RoleActor), crewCredits(details.Credits.Crew)...),
	}
}

// seasonRecord maps a season without its episodes. Seasons and episodes carry no provider
// IDs: their TMDB IDs live in other ID spaces than the show's, and enrichment finds them
// through the show.
func seasonRecord(season seasonDetails) metadata.Record {
	record := metadata.Record{Title: season.Name, Summary: season.Overview}
	record.ReleaseDate, record.Year = parseDate(season.AirDate)

	return record
}

func episodeRecord(episode episodeDetails) metadata.Record {
	record := metadata.Record{Title: episode.Name, Summary: episode.Overview}
	record.ReleaseDate, record.Year = parseDate(episode.AirDate)
	record.People = append(crewCredits(episode.Crew), castCredits(episode.GuestStars, metadata.RoleGuestStar)...)

	return record
}

// parseDate reads TMDB's "2006-01-02"; an empty or malformed date is no value.
func parseDate(text string) (metadata.Date, int) {
	date, err := metadata.ParseDate(text)
	if err != nil {
		return metadata.Date{}, 0
	}

	return date, date.Year()
}

func names(entries []namedEntry) []string {
	var result []string

	for _, entry := range entries {
		if name := strings.TrimSpace(entry.Name); name != "" {
			result = append(result, name)
		}
	}

	return result
}

func castCredits(cast []castMember, role metadata.Role) []metadata.Credit {
	var result []metadata.Credit

	for _, member := range cast {
		if len(result) == maxCast {
			break
		}

		if name := strings.TrimSpace(member.Name); name != "" {
			result = append(result, metadata.Credit{Name: name, Role: role, Character: member.Character, Order: member.Order})
		}
	}

	return result
}

// crewCredits keeps directors, writers and producers once each, in TMDB's order.
func crewCredits(crew []crewMember) []metadata.Credit {
	var result []metadata.Credit

	seen := map[metadata.Credit]bool{}

	for _, member := range crew {
		role, ok := crewRole(member)
		credit := metadata.Credit{Name: strings.TrimSpace(member.Name), Role: role, Order: len(result)}
		key := metadata.Credit{Name: credit.Name, Role: role}

		if !ok || credit.Name == "" || seen[key] {
			continue
		}

		seen[key] = true

		result = append(result, credit)
	}

	return result
}

func crewRole(member crewMember) (metadata.Role, bool) {
	switch {
	case member.Job == jobDirector:
		return metadata.RoleDirector, true
	case member.Department == departmentWriting:
		return metadata.RoleWriter, true
	case member.Job == jobProducer:
		return metadata.RoleProducer, true
	default:
		return "", false
	}
}

// providerIDs lists the TMDB ID and the valid external IDs.
func providerIDs(tmdbID int64, external externalIDs) []metadata.ProviderID {
	candidates := []metadata.ProviderID{{Provider: metadata.ProviderTMDB, Value: formatID(tmdbID)}}
	candidates = append(candidates,
		metadata.ProviderID{Provider: metadata.ProviderIMDb, Value: external.IMDbID},
		metadata.ProviderID{Provider: metadata.ProviderTVDB, Value: formatID(external.TVDBID)},
	)

	var result []metadata.ProviderID

	for _, id := range candidates {
		if metadata.ValidateProviderID(id.Provider, id.Value) == nil {
			result = append(result, id)
		}
	}

	return result
}

func formatID(id int64) string {
	if id <= 0 {
		return ""
	}

	return strconv.FormatInt(id, 10)
}

// movieCertification returns the first certification of the region, else of the US.
func movieCertification(dates releaseDates, region string) string {
	for _, wanted := range []string{region, fallbackRegion} {
		for _, result := range dates.Results {
			if result.Region != wanted {
				continue
			}

			for _, release := range result.ReleaseDates {
				if certification := strings.TrimSpace(release.Certification); certification != "" {
					return certification
				}
			}
		}
	}

	return ""
}

// showRating returns the rating of the region, else of the US.
func showRating(ratings contentRatings, region string) string {
	for _, wanted := range []string{region, fallbackRegion} {
		for _, result := range ratings.Results {
			if rating := strings.TrimSpace(result.Rating); result.Region == wanted && rating != "" {
				return rating
			}
		}
	}

	return ""
}

// fillEmpty copies the fields TMDB left untranslated from the English record.
func fillEmpty(record, english metadata.Record) metadata.Record {
	if record.Title == "" {
		record.Title = english.Title
	}

	if record.Summary == "" {
		record.Summary = english.Summary
	}

	if record.Tagline == "" {
		record.Tagline = english.Tagline
	}

	return record
}

// needsFallback reports whether a translated record misses a field English may have.
func needsFallback(record metadata.Record) bool {
	return record.Title == "" || record.Summary == ""
}

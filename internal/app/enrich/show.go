package enrich

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// resolveShowChild enriches a season or an episode through its show: seasons and episodes
// are never searched. Their numbers come from the placement (path); an episode the provider
// does not list keeps its state (an unmatched one is written as unmatched). Without a show
// ID nothing is asked: the show's own enrichment comes first.
func (enricher *Enricher) resolveShowChild(ctx context.Context, current item) (resolution, error) {
	showRef, ok := refFromIDs(current.snapshot.ParentProviderIDs, finder{
		kind:      match.RefKindShow,
		providers: []match.RefProvider{match.RefTMDB, match.RefIMDb},
	})
	if !ok {
		return resolution{skip: true}, nil
	}

	season, err := enricher.deps.Shows.SeasonDetails(ctx, SeasonRequest{
		Show:     showRef,
		Season:   current.snapshot.SeasonNumber,
		Language: current.language,
	})
	if err != nil {
		return resolution{}, fmt.Errorf("season %d of %s show %s: %w",
			current.snapshot.SeasonNumber, showRef.Provider, showRef.ID, err)
	}

	details, found := childDetails(current.snapshot, season)
	if !found {
		// Like match.Resolve, a lookup that finds nothing never drops a match the item has.
		state := match.MatchStateUnmatched
		if current.snapshot.MatchState == match.MatchStateMatched {
			state = match.MatchStateMatched
		}

		return resolution{result: MatchResult{State: state}}, nil
	}

	return resolution{
		result: MatchResult{
			State: match.MatchStateMatched, Provider: showRef.Provider, Ref: showRef, Confidence: exactConfidence,
		},
		source:  metadata.SourceTMDB,
		details: &details,
	}, nil
}

// childDetails picks the season itself or the episode with the placement's number.
func childDetails(snapshot ItemSnapshot, season SeasonDetails) (Details, bool) {
	if snapshot.Kind != library.ItemKindEpisode {
		return season.Details, true
	}

	for _, episode := range season.Episodes {
		if episode.Number == snapshot.EpisodeNumber {
			return episode.Details, true
		}
	}

	return Details{}, false
}

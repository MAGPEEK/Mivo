package restapi

import (
	"context"
	"errors"
	"math"
	"net/http"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/app/identify"
	"github.com/magpeek/mivo/internal/app/metadatasettings"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// CodeNotImplemented is the problem code of operations or parameter values whose contract
// exists but whose handler is not wired (the online metadata endpoints of an API built
// without MetadataServices; the rows endpoints of an API wired without a rows service). It
// is published since API 1.5.0 so clients can show "not available on this server" instead
// of a generic error.
const CodeNotImplemented = "not_implemented"

// Problem codes of the online metadata endpoints (api/openapi.yaml, Problem.code).
const (
	// CodeOnlineDisabled reports that online metadata is switched off globally, for the
	// library or for the item's provider (409).
	CodeOnlineDisabled = "online_disabled"
	// CodeProviderUnavailable reports a provider that cannot be reached or is rate limiting
	// (503 with Retry-After and retryAfterSeconds).
	CodeProviderUnavailable = "provider_unavailable"
)

// ProviderRetryAfterSeconds is the Retry-After of provider_unavailable (also of the poster
// proxy) when the provider named no pause: the outbound breaker and rate limiter recover
// within this time.
const ProviderRetryAfterSeconds int32 = 30

// notImplementedProblem is the 501 answer of an operation without its service.
func notImplementedProblem() apigen.Problem {
	return NewProblem(http.StatusNotImplemented, CodeNotImplemented)
}

// IdentifyService searches candidates, applies and removes matches. Implemented by
// *identify.Service.
type IdentifyService interface {
	// Search returns candidates best first. Errors wrap domain.ErrInvalid, ErrConflict
	// (online off), ErrNotFound, ErrUnavailable or ErrRateLimited.
	Search(ctx context.Context, itemID string, query identify.Query) ([]match.Scored, error)
	// Apply links the item to a provider entry and queues its enrichment.
	Apply(ctx context.Context, itemID string, provider match.RefProvider, externalID string) error
	// Unmatch removes the online match of an item.
	Unmatch(ctx context.Context, itemID string) error
}

// MetadataRefresher queues refreshes. Implemented by *identify.Service.
type MetadataRefresher interface {
	// RefreshItem queues the item (and a show's seasons and episodes).
	RefreshItem(ctx context.Context, itemID string) error
	// RefreshLibrary queues every matchable item of the library.
	RefreshLibrary(ctx context.Context, libraryID string) error
}

// MetadataSettingsService reads and changes the online metadata settings. Implemented by
// *metadatasettings.Service; it never returns a key.
type MetadataSettingsService interface {
	// Settings returns the current settings.
	Settings(ctx context.Context) (metadatasettings.Settings, error)
	// Update applies a change; malformed values wrap a *metadatasettings.FieldError.
	Update(ctx context.Context, change metadatasettings.Change) (metadatasettings.Settings, error)
}

// MetadataStatusSource reports the enrichment queue. Implemented by *enrich.Queue.
type MetadataStatusSource interface {
	// Status returns the queue length and the provider states.
	Status(ctx context.Context) (enrich.Status, error)
}

// MatchStatusReader reads the match of several items at once. Implemented by the
// enrichment store of adapters/sqlite; items without a stored match are left out.
type MatchStatusReader interface {
	// ItemMatchStatuses returns the stored match statuses by item ID.
	ItemMatchStatuses(ctx context.Context, itemIDs []string) (map[string]match.ItemStatus, error)
}

// CandidatePosters turns a provider poster into a Mivo URL the browser may load (ADR 0010).
// Implemented by the poster proxy of adapters/http/posterproxy.
type CandidatePosters interface {
	// PosterURL registers the picture and returns its server URL, "" when it cannot be
	// offered (for example a picture of a provider the proxy does not fetch from).
	PosterURL(ref enrich.ArtworkRef) string
}

// MetadataServices are the optional services of the online metadata endpoints (milestone
// 6). An operation whose service is nil answers 501 "not_implemented"; without Matches the
// item answers omit matchState and metadataSources, without Posters the candidates omit
// posterUrl.
type MetadataServices struct {
	Identify IdentifyService
	Refresh  MetadataRefresher
	Settings MetadataSettingsService
	Status   MetadataStatusSource
	Matches  MatchStatusReader
	Posters  CandidatePosters
}

// onlineProblem maps the failures the online endpoints answer with their own codes:
// ErrConflict is online_disabled (409); a failed provider call (identify.ErrProvider) that
// wraps ErrRateLimited or ErrUnavailable is provider_unavailable (503) with the seconds after
// which a retry may succeed. Any other ErrUnavailable, such as a shutdown, keeps the generic
// 503 "unavailable" without Retry-After. It reports false for every other error.
func onlineProblem(err error) (apigen.Problem, bool) {
	switch {
	case errors.Is(err, domain.ErrConflict):
		return NewProblem(http.StatusConflict, CodeOnlineDisabled), true
	case errors.Is(err, identify.ErrProvider) && (errors.Is(err, domain.ErrRateLimited) || errors.Is(err, domain.ErrUnavailable)):
		problem := NewProblem(http.StatusServiceUnavailable, CodeProviderUnavailable)
		seconds := retryAfterSeconds(err)
		problem.RetryAfterSeconds = &seconds

		return problem, true
	default:
		return apigen.Problem{}, false
	}
}

// retryAfterSeconds is the pause a rate-limited provider asked for, rounded up to whole
// seconds, or ProviderRetryAfterSeconds.
func retryAfterSeconds(err error) int32 {
	var retryAfter enrich.RetryAfterError
	if !errors.As(err, &retryAfter) || retryAfter.RetryAfter() <= 0 {
		return ProviderRetryAfterSeconds
	}

	seconds := math.Ceil(retryAfter.RetryAfter().Seconds())

	return int32(min(seconds, float64(time.Hour/time.Second)))
}

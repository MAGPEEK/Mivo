package nfo

import (
	"net/url"
	"strings"

	"github.com/magpeek/mivo/internal/domain/metadata"
)

// Hosts and path segments of the provider URL forms a URL NFO may contain.
const (
	hostTMDB          = "themoviedb.org"
	hostIMDb          = "imdb.com"
	hostTVDB          = "thetvdb.com"
	segmentTMDBMovie  = "movie"
	segmentTMDBShow   = "tv"
	segmentIMDbTitle  = "title"
	segmentTVDBSeries = "series"
	queryTVDBID       = "id"
	imdbPrefix        = "tt"
)

// providerOrder is the stable output order of provider IDs.
func providerOrder() []metadata.Provider {
	return []metadata.Provider{metadata.ProviderTMDB, metadata.ProviderIMDb, metadata.ProviderTVDB}
}

// providerCollector keeps one ID per provider: the first valid one, unless a later one is
// marked default="true" and the kept one is not.
type providerCollector struct {
	values    map[metadata.Provider]string
	isDefault map[metadata.Provider]bool
	invalid   bool
}

func newProviderCollector() *providerCollector {
	return &providerCollector{values: map[metadata.Provider]string{}, isDefault: map[metadata.Provider]bool{}}
}

func (collector *providerCollector) add(provider metadata.Provider, value string, isDefault bool) {
	if value == "" {
		return
	}

	if err := metadata.ValidateProviderID(provider, value); err != nil {
		collector.invalid = true

		return
	}

	_, exists := collector.values[provider]
	if exists && (collector.isDefault[provider] || !isDefault) {
		return
	}

	collector.values[provider] = value
	collector.isDefault[provider] = isDefault
}

func (collector *providerCollector) ids() []metadata.ProviderID {
	var ids []metadata.ProviderID

	for _, provider := range providerOrder() {
		if value, found := collector.values[provider]; found {
			ids = append(ids, metadata.ProviderID{Provider: provider, Value: value})
		}
	}

	return ids
}

// readProviderIDs reads <uniqueid> first, then the older <tmdbid>, <imdbid>, <tvdbid> and
// <id> elements.
func (builder *recordBuilder) readProviderIDs() []metadata.ProviderID {
	collector := newProviderCollector()

	for _, element := range builder.children("uniqueid") {
		provider, err := metadata.ParseProvider(strings.ToLower(strings.TrimSpace(element.attribute("type"))))
		if err != nil {
			continue // IDs of other databases (anidb, …) are not used in milestone 3
		}

		isDefault := strings.EqualFold(strings.TrimSpace(element.attribute("default")), "true")
		collector.add(provider, cleanText(element.text.String()), isDefault)
	}

	collector.add(metadata.ProviderTMDB, builder.firstText("tmdbid"), false)
	collector.add(metadata.ProviderIMDb, builder.firstText("imdbid"), false)
	collector.add(metadata.ProviderTVDB, builder.firstText("tvdbid"), false)

	if id := builder.firstText("id"); id != "" {
		collector.add(builder.legacyIDProvider(id), id, false)
	}

	if collector.invalid {
		builder.warn(WarningInvalidProviderID)
	}

	return collector.ids()
}

// legacyIDProvider interprets Kodi's untyped <id>: IMDb IDs carry their "tt" prefix, numeric
// IDs come from TMDB for movies and from TVDB for shows, seasons and episodes.
func (builder *recordBuilder) legacyIDProvider(id string) metadata.Provider {
	switch {
	case strings.HasPrefix(id, imdbPrefix):
		return metadata.ProviderIMDb
	case builder.kind == KindMovie:
		return metadata.ProviderTMDB
	default:
		return metadata.ProviderTVDB
	}
}

// mergeProviderIDs adds the extra IDs for providers the record has no ID for yet.
func mergeProviderIDs(ids, extra []metadata.ProviderID) []metadata.ProviderID {
	collector := newProviderCollector()

	for _, id := range append(ids, extra...) {
		collector.add(id.Provider, id.Value, false)
	}

	return collector.ids()
}

// providerIDsFromURLs finds provider IDs in the URLs of free text. The URLs are only parsed,
// never requested (ADR 0027 §7).
func providerIDsFromURLs(text string) []metadata.ProviderID {
	collector := newProviderCollector()

	for _, word := range strings.Fields(text) {
		provider, id := providerIDFromURL(word)
		if provider != "" {
			collector.add(provider, id, false)
		}
	}

	return collector.ids()
}

func providerIDFromURL(text string) (metadata.Provider, string) {
	if !strings.Contains(text, "://") {
		text = "https://" + text
	}

	parsed, err := url.Parse(text)
	if err != nil {
		return "", ""
	}

	host := strings.ToLower(parsed.Hostname())
	segments := strings.FieldsFunc(parsed.Path, func(character rune) bool { return character == '/' })

	switch {
	case hasHost(host, hostTMDB):
		return metadata.ProviderTMDB, tmdbIDFromPath(segments)
	case hasHost(host, hostIMDb):
		return metadata.ProviderIMDb, imdbIDFromPath(segments)
	case hasHost(host, hostTVDB):
		return metadata.ProviderTVDB, tvdbIDFromURL(parsed, segments)
	default:
		return "", ""
	}
}

func hasHost(host, domainName string) bool {
	return host == domainName || strings.HasSuffix(host, "."+domainName)
}

// tmdbIDFromPath reads /movie/<id> and /tv/<id>; TMDB appends "-slug" to the ID.
func tmdbIDFromPath(segments []string) string {
	if len(segments) < 2 || (segments[0] != segmentTMDBMovie && segments[0] != segmentTMDBShow) {
		return ""
	}

	id, _, _ := strings.Cut(segments[1], "-")

	return id
}

func imdbIDFromPath(segments []string) string {
	if len(segments) < 2 || segments[0] != segmentIMDbTitle {
		return ""
	}

	return segments[1]
}

// tvdbIDFromURL reads the old "?id=<id>" form and numeric /series/<id> paths.
func tvdbIDFromURL(parsed *url.URL, segments []string) string {
	if id := parsed.Query().Get(queryTVDBID); id != "" {
		return id
	}

	if len(segments) >= 2 && segments[0] == segmentTVDBSeries {
		return segments[1]
	}

	return ""
}

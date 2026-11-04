// Package nfo reads Kodi NFO files (ADR 0027 §7) into metadata records. It works on bytes
// only: the caller reads the file through the library's os.Root, and nothing an NFO
// references (artwork or actor thumbnail URLs) is ever fetched (ADR 0010, ADR 0022).
package nfo

import (
	"bytes"
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// Limits that keep a crafted NFO from exhausting memory or CPU. Real NFO files are a few
// kilobytes with a nesting depth of three and well under a thousand elements.
const (
	// MaxInputBytes is the largest NFO the parser accepts (ADR 0027 §7).
	MaxInputBytes = 1 << 20
	// maxElementDepth bounds the nesting of elements below the document.
	maxElementDepth = 16
	// maxElementCount bounds the number of elements in one document.
	maxElementCount = 20000
)

// Warning codes. They name what was dropped, never the file content.
const (
	WarningInvalidProviderID = "invalid_provider_id"
	WarningInvalidYear       = "invalid_year"
	WarningInvalidDate       = "invalid_date"
	WarningInvalidOrder      = "invalid_actor_order"
)

// Kind is the kind of item an NFO describes; it selects the expected root element.
type Kind string

// Known kinds.
const (
	KindMovie   Kind = "movie"
	KindShow    Kind = "show"
	KindSeason  Kind = "season"
	KindEpisode Kind = "episode"
)

// rootElement returns the root element name of the kind, or "" for an unknown kind.
func (kind Kind) rootElement() string {
	switch kind {
	case KindMovie:
		return "movie"
	case KindShow:
		return "tvshow"
	case KindSeason:
		return "season"
	case KindEpisode:
		return "episodedetails"
	default:
		return ""
	}
}

// Result is what one NFO file says: one record, or one per <episodedetails> root of a
// multi-episode file, and the codes of the values that were dropped.
type Result struct {
	Records  []metadata.Record
	Warnings []string
}

// utf8ByteOrderMark is the optional UTF-8 byte order mark at the start of a file.
const utf8ByteOrderMark = "\xef\xbb\xbf"

// Parse reads an NFO of the given kind. A file that is too large, empty, malformed, in an
// unsupported encoding or without any usable content fails with domain.ErrUnsupported; the
// error names the reason but never the file content.
func Parse(data []byte, kind Kind) (Result, error) {
	root := kind.rootElement()
	if root == "" {
		return Result{}, fmt.Errorf("parse nfo: unknown kind %q: %w", kind, domain.ErrInvalid)
	}

	if len(data) > MaxInputBytes {
		return Result{}, unsupported("file larger than %d bytes", MaxInputBytes)
	}

	data = bytes.TrimPrefix(data, []byte(utf8ByteOrderMark))
	trimmed := bytes.TrimSpace(data)

	if len(trimmed) == 0 {
		return Result{}, unsupported("empty file")
	}

	if trimmed[0] != '<' {
		return parseURLOnly(string(trimmed))
	}

	return parseXML(data, kind)
}

// parseURLOnly reads a "URL NFO": text that only names the item's page at a provider.
func parseURLOnly(text string) (Result, error) {
	ids := providerIDsFromURLs(text)
	if len(ids) == 0 {
		return Result{}, unsupported("no XML and no provider URL")
	}

	return Result{Records: []metadata.Record{{ProviderIDs: ids}}}, nil
}

// parseXML reads the XML roots of the kind and any provider URL that follows them.
func parseXML(data []byte, kind Kind) (Result, error) {
	text, err := toUTF8(data)
	if err != nil {
		return Result{}, err
	}

	document, err := decodeDocument(text)
	if err != nil {
		return Result{}, err
	}

	roots := document.rootsNamed(kind.rootElement())
	if len(roots) == 0 {
		return Result{}, unsupported("no <%s> root element", kind.rootElement())
	}

	if kind != KindEpisode {
		roots = roots[:1]
	}

	var result Result

	for _, root := range roots {
		record, warnings := buildRecord(root, kind)
		result.Records = append(result.Records, record)
		result.Warnings = append(result.Warnings, warnings...)
	}

	trailingIDs := providerIDsFromURLs(document.trailingText)
	result.Records[0].ProviderIDs = mergeProviderIDs(result.Records[0].ProviderIDs, trailingIDs)

	return result, nil
}

func unsupported(format string, arguments ...any) error {
	return fmt.Errorf("parse nfo: "+format+": %w", append(arguments, domain.ErrUnsupported)...)
}

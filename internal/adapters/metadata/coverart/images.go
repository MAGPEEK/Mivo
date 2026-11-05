package coverart

import (
	"net/url"
	"slices"
	"strings"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

const (
	// maxCovers bounds the references offered per album; the first is the selection.
	maxCovers = 5

	mbidLength = 36

	schemeHTTP  = "http"
	schemeHTTPS = "https"
)

// mbidDashPositions are the hyphen offsets of a MusicBrainz ID (a UUID, 8-4-4-4-12).
var mbidDashPositions = [...]int{8, 13, 18, 23} //nolint:gochecknoglobals // read-only lookup table

// preferredThumbnails are the sizes tried per image, largest first (ADR 0031 §11). "large"
// is the old name of the 500 px thumbnail. The original image is only the last resort
// because it can be far larger; the artwork fetcher caps it at 15 MiB.
var preferredThumbnails = [...]string{"1200", "500", "large"} //nolint:gochecknoglobals // read-only lookup table

// imageListing is the Cover Art Archive answer for a release or release group.
type imageListing struct {
	Images []coverImage `json:"images"`
}

type coverImage struct {
	Approved   bool              `json:"approved"`
	Front      bool              `json:"front"`
	Image      string            `json:"image"`
	Thumbnails map[string]string `json:"thumbnails"`
}

// frontCovers turns the approved front images into poster references. Back covers and other
// image types are left out until the artwork model has a kind for them (M3-13 adds square).
func frontCovers(listing imageListing) []enrich.ArtworkRef {
	var refs []enrich.ArtworkRef

	for _, image := range listing.Images {
		if len(refs) == maxCovers {
			break
		}

		if !image.Approved || !image.Front {
			continue
		}

		address, ok := bestURL(image)
		if !ok || slices.ContainsFunc(refs, func(ref enrich.ArtworkRef) bool { return ref.Path == address }) {
			continue
		}

		refs = append(refs, enrich.ArtworkRef{
			Provider: enrich.ArtworkProviderCoverArt,
			Kind:     artwork.KindPoster,
			Path:     address,
		})
	}

	return refs
}

// bestURL returns the first usable address among the preferred thumbnails and the original.
func bestURL(image coverImage) (string, bool) {
	candidates := make([]string, 0, len(preferredThumbnails)+1)
	for _, size := range preferredThumbnails {
		candidates = append(candidates, image.Thumbnails[size])
	}

	candidates = append(candidates, image.Image)

	for _, candidate := range candidates {
		if address, ok := safeImageURL(candidate); ok {
			return address, true
		}
	}

	return "", false
}

// safeImageURL accepts a plain coverartarchive.org address and returns it with HTTPS: the
// archive still lists http:// URLs, and the artwork fetcher only downloads HTTPS. Anything
// naming another host, credentials, a port, a query, a fragment, escapes or dot segments is
// refused, so the reference passes the fetcher's checks unchanged.
func safeImageURL(rawURL string) (string, bool) {
	if rawURL == "" {
		return "", false
	}

	target, err := url.Parse(rawURL)
	if err != nil || (target.Scheme != schemeHTTP && target.Scheme != schemeHTTPS) {
		return "", false
	}

	if !strings.EqualFold(target.Host, apiHost) || target.User != nil || target.RawQuery != "" ||
		target.Fragment != "" || target.RawPath != "" || target.Opaque != "" || hasDotSegment(target.Path) {
		return "", false
	}

	target.Scheme = schemeHTTPS
	target.Host = apiHost

	return target.String(), true
}

func hasDotSegment(urlPath string) bool {
	return slices.ContainsFunc(strings.Split(urlPath, "/"), func(segment string) bool {
		return segment == "." || segment == ".." || strings.Contains(segment, "\\")
	})
}

// normalizedMBID lowercases id and reports whether it has the shape of a MusicBrainz ID, so
// nothing but hex digits and hyphens reaches the request path.
func normalizedMBID(id string) (string, bool) {
	id = strings.ToLower(id)
	if len(id) != mbidLength {
		return "", false
	}

	for index := range len(id) {
		if slices.Contains(mbidDashPositions[:], index) {
			if id[index] != '-' {
				return "", false
			}

			continue
		}

		if !isHexDigit(id[index]) {
			return "", false
		}
	}

	return id, true
}

func isHexDigit(character byte) bool {
	return (character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')
}

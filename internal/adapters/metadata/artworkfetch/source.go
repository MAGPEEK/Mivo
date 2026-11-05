package artworkfetch

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

const (
	schemeHTTPS = "https"
	schemeHTTP  = "http"
	// maxTMDBFileName bounds a TMDB file name; real ones are about 32 characters.
	maxTMDBFileName = 128
	refTagSeparator = "\x00"
)

// TMDBHosts returns the hosts TMDB pictures are fetched from (ADR 0031 §1). The outbound
// client for TMDB should use it for AllowedHosts and RedirectHosts.
func TMDBHosts() []string { return []string{"image.tmdb.org"} }

// CoverArtHosts returns the hosts Cover Art Archive references may name (ADR 0031 §1).
func CoverArtHosts() []string { return []string{"coverartarchive.org"} }

// CoverArtRedirectHosts returns where a Cover Art Archive request may be redirected to; the
// outbound client for Cover Art Archive should use it for RedirectHosts.
func CoverArtRedirectHosts() []string { return []string{".archive.org"} }

// refTag names the stored file of a reference. It depends on the provider reference only,
// so a re-fetch finds the file without a download and no part of the URL reaches the path.
// The kind is part of it because the URL builder may pick a different size per kind.
func refTag(ref enrich.ArtworkRef) string {
	identity := strings.Join([]string{string(ref.Provider), string(ref.Kind), ref.Path}, refTagSeparator)
	digest := sha256.Sum256([]byte(identity))

	return hex.EncodeToString(digest[:])[:artwork.TagLength]
}

// checkTMDBPath accepts a TMDB file path ("/abc123.jpg"): a single plain file name, so a
// URL builder appending it to a base path cannot climb out of it.
func checkTMDBPath(filePath string) error {
	name, ok := strings.CutPrefix(filePath, "/")
	if !ok || !isPlainFileName(name) {
		return fmt.Errorf("tmdb picture path %q: %w", filePath, domain.ErrInvalid)
	}

	return nil
}

// pathAsURL is the URL builder of providers whose reference path is the URL.
func pathAsURL(ref enrich.ArtworkRef) (string, error) {
	return ref.Path, nil
}

func isPlainFileName(name string) bool {
	stem, extension, found := strings.Cut(name, ".")
	if !found || stem == "" || len(name) > maxTMDBFileName {
		return false
	}

	if !slices.Contains([]string{"jpg", "jpeg", "png", "webp"}, strings.ToLower(extension)) {
		return false
	}

	for _, char := range stem {
		if !isFileNameChar(char) {
			return false
		}
	}

	return true
}

func isFileNameChar(char rune) bool {
	return (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
		(char >= '0' && char <= '9') || char == '-' || char == '_'
}

// checkedURL parses an absolute picture URL and accepts it only when it uses HTTPS (plain
// HTTP only with allowInsecure, for tests), names an allowed host exactly and carries no
// credentials, query, fragment, escaped characters or dot segments.
func checkedURL(rawURL string, allowedHosts []string, allowInsecure bool) (string, error) {
	target, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("picture url: %w", domain.ErrInvalid)
	}

	if !schemeAllowed(target.Scheme, allowInsecure) {
		return "", fmt.Errorf("picture url scheme %q: %w", target.Scheme, domain.ErrInvalid)
	}

	if !slices.Contains(allowedHosts, strings.ToLower(target.Hostname())) {
		return "", fmt.Errorf("picture host %q: %w", target.Hostname(), domain.ErrForbidden)
	}

	if target.User != nil || target.RawQuery != "" || target.Fragment != "" || target.RawPath != "" ||
		target.Opaque != "" || hasDotSegment(target.Path) {
		return "", fmt.Errorf("picture url %s: %w", target.Redacted(), domain.ErrInvalid)
	}

	return target.String(), nil
}

func schemeAllowed(scheme string, allowInsecure bool) bool {
	return scheme == schemeHTTPS || (allowInsecure && scheme == schemeHTTP)
}

func hasDotSegment(urlPath string) bool {
	return slices.ContainsFunc(strings.Split(urlPath, "/"), func(segment string) bool {
		return segment == "." || segment == ".." || strings.Contains(segment, "\\")
	})
}

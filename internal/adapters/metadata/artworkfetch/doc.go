// Package artworkfetch downloads provider pictures (TMDB images, Cover Art Archive covers)
// into the data directory (ADR 0031 §11). It implements enrich.ArtworkDownloader.
//
// A picture is fetched once: its file name is a tag derived from the provider reference
// only, never from the URL, and the bytes are checked (JPEG, PNG or WebP by content, size
// cap, sane dimensions, not truncated) before they are written atomically below
// <data>/artwork. The host allow-list is checked here and again by the outbound client; the
// client's SSRF guard is the second line of defence.
package artworkfetch

// Package artwork holds the pure rules for the pictures of an item (ADR 0027 §8, UX ED-05,
// LB-44): artwork kinds and sources, which local file names count as which kind, how one
// artwork per kind is selected, the allowed image widths and output formats, and the cache
// tag. It also defines the value types shared by the image renderer, the image use case and
// the catalog reader (Stored, RenderRequest, Rendered), so those can be built in parallel.
//
// The package reads no file and decodes no image; adapters list sidecars, read file states
// and render with FFmpeg.
package artwork

package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

const (
	// manifestFileName is the manifest inside the output directory.
	manifestFileName = "manifest.json"
	// manifestSchemaVersion changes when consumers have to adapt to the JSON shape.
	manifestSchemaVersion = 1
)

// entryStatus is the outcome of one case.
type entryStatus string

const (
	statusGenerated entryStatus = "generated"
	statusSkipped   entryStatus = "skipped"
	statusFailed    entryStatus = "failed"
)

// manifest lists every case of a run. Integration tests read it to find corpus files.
type manifest struct {
	SchemaVersion int             `json:"schemaVersion"`
	FFmpegVersion string          `json:"ffmpegVersion"`
	Cases         []manifestEntry `json:"cases"`
}

// manifestEntry is the result of one case together with its expected properties.
type manifestEntry struct {
	Name     string      `json:"name"`
	Category string      `json:"category"`
	File     string      `json:"file"`
	Status   entryStatus `json:"status"`
	// Reason explains skipped and failed cases and is empty for generated ones.
	Reason   string      `json:"reason"`
	Expected expectation `json:"expected"`
}

func newEntry(c mediaCase, status entryStatus, reason string) manifestEntry {
	return manifestEntry{
		Name:     c.Name,
		Category: c.Category,
		File:     c.Output,
		Status:   status,
		Reason:   reason,
		Expected: c.expected(),
	}
}

// newManifest builds a manifest whose entries are sorted by name, so the file is stable
// regardless of the order in which parallel jobs finished.
func newManifest(ffmpegVersion string, entries []manifestEntry) manifest {
	// A non-nil slice encodes as [] rather than null when nothing was selected.
	sorted := append(make([]manifestEntry, 0, len(entries)), entries...)
	slices.SortFunc(sorted, func(a, b manifestEntry) int { return cmp.Compare(a.Name, b.Name) })

	return manifest{SchemaVersion: manifestSchemaVersion, FFmpegVersion: ffmpegVersion, Cases: sorted}
}

// count returns how many entries have the given status.
func (m manifest) count(status entryStatus) int {
	total := 0

	for _, entry := range m.Cases {
		if entry.Status == status {
			total++
		}
	}

	return total
}

// encode returns the indented JSON with a trailing newline.
func (m manifest) encode() ([]byte, error) {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode manifest: %w", err)
	}

	return append(data, '\n'), nil
}

// writeManifest replaces <dir>/manifest.json atomically, so readers never see half a file.
func writeManifest(dir string, m manifest) error {
	data, err := m.encode()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, outputDirPermissions); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	temp, err := os.CreateTemp(dir, manifestFileName+"-*"+partialSuffix)
	if err != nil {
		return fmt.Errorf("create manifest: %w", err)
	}

	_, writeErr := temp.Write(data)
	closeErr := temp.Close()

	if err := errors.Join(writeErr, closeErr); err != nil {
		return errors.Join(fmt.Errorf("write manifest: %w", err), removePartial(temp.Name()))
	}

	if err := os.Rename(temp.Name(), filepath.Join(dir, manifestFileName)); err != nil {
		return errors.Join(fmt.Errorf("replace manifest: %w", err), removePartial(temp.Name()))
	}

	return nil
}

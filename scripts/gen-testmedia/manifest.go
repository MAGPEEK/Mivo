package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

const (
	// manifestFileName is the manifest inside the output directory.
	manifestFileName = "manifest.json"
	// manifestSchemaVersion changes when consumers have to adapt to the JSON shape.
	// Version 2: audio is a list of streams, subtitles, chapters, tags, sidecars, skipKind
	// and the encoders used were added.
	manifestSchemaVersion = 2
	// outputFilePermissions keeps corpus files readable for the owner and group, matching
	// outputDirPermissions.
	outputFilePermissions = 0o640
)

// entryStatus is the outcome of one case.
type entryStatus string

const (
	statusGenerated entryStatus = "generated"
	statusSkipped   entryStatus = "skipped"
	statusFailed    entryStatus = "failed"
)

// skipKind says why a case was skipped.
type skipKind string

const (
	// skipMissingCapability: the local FFmpeg lacks an encoder or muxer the case needs.
	skipMissingCapability skipKind = "missing-capability"
	// skipNotSelected: the case was excluded by -only and no file exists from earlier runs.
	skipNotSelected skipKind = "not-selected"
	// skipKnownGap: no FFmpeg build can produce the format (ADR 0021).
	skipKnownGap skipKind = "known-gap"
	// skipInterrupted: the run was interrupted before the case was built.
	skipInterrupted skipKind = "interrupted"
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
	// SkipKind classifies skipped cases and is empty otherwise.
	SkipKind skipKind `json:"skipKind,omitempty"`
	// Reason explains skipped and failed cases and is empty for generated ones.
	Reason string `json:"reason"`
	// Encoders are the FFmpeg encoders that built the file. A file kept from an earlier run
	// keeps the encoders of the run that built it, taken from the previous manifest, because
	// this run might choose another alternative. They are empty for cases without a file and
	// for kept files the previous manifest does not describe.
	Encoders []string    `json:"encoders,omitempty"`
	Expected expectation `json:"expected"`

	// kept marks a generated entry whose file an earlier run built. It only labels progress
	// and summary; consumers of the manifest see a generated file either way.
	kept bool
}

// keptLabel names files kept from an earlier run in progress and summary.
const keptLabel = "kept"

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

// newBuiltEntry is a case this run built with the encoders of the chosen alternative.
func newBuiltEntry(c mediaCase) manifestEntry {
	entry := newEntry(c, statusGenerated, "")
	entry.Encoders = slices.Clone(c.Encoders)

	return entry
}

// newKeptEntry is a case whose file an earlier run built with the given encoders.
func newKeptEntry(c mediaCase, encoders []string) manifestEntry {
	entry := newEntry(c, statusGenerated, "")
	entry.Encoders = slices.Clone(encoders)
	entry.kept = true

	return entry
}

// progressLabel is the status shown in progress lines: "kept" for files of earlier runs.
func (e manifestEntry) progressLabel() string {
	if e.kept {
		return keptLabel
	}

	return string(e.Status)
}

func newSkippedEntry(c mediaCase, kind skipKind, reason string) manifestEntry {
	entry := newEntry(c, statusSkipped, reason)
	entry.SkipKind = kind

	return entry
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

// previousBuild is what the previous manifest says about a generated file.
type previousBuild struct {
	file     string
	encoders []string
}

// previousEncoders maps case names to the files earlier runs built.
type previousEncoders map[string]previousBuild

// of returns the encoders that built the file of c, or nil when the previous manifest did not
// list that file as generated, e.g. because the case was renamed or wrote another file.
func (p previousEncoders) of(c mediaCase) []string {
	build, found := p[c.Name]
	if !found || build.file != c.Output {
		return nil
	}

	return build.encoders
}

// readPreviousEncoders reads the generated entries of <dir>/manifest.json. A missing manifest
// is no error; it simply describes nothing.
func readPreviousEncoders(dir string) (previousEncoders, error) {
	data, err := fs.ReadFile(os.DirFS(dir), manifestFileName)
	if errors.Is(err, fs.ErrNotExist) {
		return previousEncoders{}, nil
	}

	if err != nil {
		return previousEncoders{}, fmt.Errorf("read %s: %w", manifestFileName, err)
	}

	var previous manifest
	if err := json.Unmarshal(data, &previous); err != nil {
		return previousEncoders{}, fmt.Errorf("decode %s: %w", manifestFileName, err)
	}

	if previous.SchemaVersion != manifestSchemaVersion {
		return previousEncoders{}, fmt.Errorf("%s has schema version %d, want %d",
			manifestFileName, previous.SchemaVersion, manifestSchemaVersion)
	}

	builds := previousEncoders{}

	for _, entry := range previous.Cases {
		if entry.Status == statusGenerated {
			builds[entry.Name] = previousBuild{file: entry.File, encoders: entry.Encoders}
		}
	}

	return builds, nil
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

	return writeFileAtomically(filepath.Join(dir, manifestFileName), data)
}

// writeFileAtomically writes data to a temporary file next to target and renames it, so
// readers never see half a file and an interrupted write never leaves a truncated target.
func writeFileAtomically(target string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(target), filepath.Base(target)+"-*"+partialSuffix)
	if err != nil {
		return fmt.Errorf("create %s: %w", filepath.Base(target), err)
	}

	_, writeErr := temp.Write(data)
	// CreateTemp uses 0600; corpus files are group-readable like their directories.
	chmodErr := temp.Chmod(outputFilePermissions)
	closeErr := temp.Close()

	if err := errors.Join(writeErr, chmodErr, closeErr); err != nil {
		return errors.Join(fmt.Errorf("write %s: %w", filepath.Base(target), err), removePartial(temp.Name()))
	}

	if err := os.Rename(temp.Name(), target); err != nil {
		return errors.Join(fmt.Errorf("replace %s: %w", filepath.Base(target), err), removePartial(temp.Name()))
	}

	return nil
}

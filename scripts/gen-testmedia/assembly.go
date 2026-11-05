package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"time"
)

var errMissingSource = errors.New("missing source")

// fileAssembly writes files in Go, optionally from the staging outputs of source cases: the
// disc trees (task M3-16i) and the photos FFmpeg cannot write with EXIF (task M3-15g).
type fileAssembly struct {
	name     string
	category string
	// label names the assembly in error messages.
	label string
	// folder is relative to the -library directory; files and the names build returns are
	// relative to it.
	folder  string
	files   []string
	sources []string
	// build returns the files from the source contents in the order of sources.
	build func(sources [][]byte) ([]assembledFile, error)
	// modified, when not zero, becomes the modification time of every written file, so a
	// photo without an EXIF capture time has a known fallback time.
	modified time.Time
}

// asCase describes the assembly for the manifest: Output is the first file and Copies are
// the others, so the case lists every file it writes. The runner never builds it.
func (assembly fileAssembly) asCase() mediaCase {
	files := make([]string, 0, len(assembly.files))
	for _, file := range assembly.files {
		files = append(files, path.Join(assembly.folder, file))
	}

	return mediaCase{Name: assembly.name, Category: assembly.category, Output: files[0], Copies: files[1:]}
}

// assembleAll writes every assembly after the source cases ran.
func (g *generator) assembleAll(ctx context.Context, assemblies []fileAssembly, filter caseFilter) []manifestEntry {
	entries := make([]manifestEntry, 0, len(assemblies))

	for _, assembly := range assemblies {
		entry := g.assemble(ctx, assembly, filter)
		g.report(entry)
		entries = append(entries, entry)
	}

	return entries
}

// assemble writes one assembly. Like an FFmpeg case, it is kept when its files exist (unless
// -force), left alone when -only does not select it, and skipped when a source is missing.
func (g *generator) assemble(ctx context.Context, assembly fileAssembly, filter caseFilter) manifestEntry {
	c := assembly.asCase()

	switch {
	case !filter.matches(c.Name):
		return g.keptOrSkippedAssembly(c, skipNotSelected, fmt.Sprintf("not selected by -only %q", filter))
	case ctx.Err() != nil:
		return g.keptOrSkippedAssembly(c, skipInterrupted, interruptedReason)
	case g.assemblyComplete(c) && !g.force:
		return newKeptEntry(c, nil)
	}

	sources, err := g.readSources(assembly.sources)
	if err != nil {
		return g.keptOrSkippedAssembly(c, skipMissingInput, err.Error())
	}

	if err := g.writeAssembly(assembly, sources); err != nil {
		return newEntry(c, statusFailed, err.Error())
	}

	return newBuiltEntry(c)
}

// keptOrSkippedAssembly lists the files of an earlier run as kept and skips them otherwise.
func (g *generator) keptOrSkippedAssembly(c mediaCase, kind skipKind, reason string) manifestEntry {
	if g.assemblyComplete(c) {
		return newKeptEntry(c, nil)
	}

	return newSkippedEntry(c, kind, reason)
}

func (g *generator) assemblyComplete(c mediaCase) bool {
	return !slices.ContainsFunc(c.files(), func(file string) bool { return !isNonEmptyFile(g.path(file)) })
}

func (g *generator) readSources(sources []string) ([][]byte, error) {
	contents := make([][]byte, 0, len(sources))

	for _, source := range sources {
		data, err := os.ReadFile(g.path(source))
		if err != nil || len(data) == 0 {
			return nil, fmt.Errorf("%w %s: the source case was not built", errMissingSource, source)
		}

		contents = append(contents, data)
	}

	return contents, nil
}

// writeAssembly builds the files and writes them. The builder must produce exactly the
// catalog's files, so the manifest never lists a file that was not written. The old files are
// removed before the first new one is written: a run that stops halfway leaves an incomplete
// assembly, which the next run rebuilds, never a mix of old and new files that counts as
// complete.
func (g *generator) writeAssembly(assembly fileAssembly, sources [][]byte) error {
	files, err := assembly.build(sources)
	if err != nil {
		return fmt.Errorf("assemble %s: %w", assembly.label, err)
	}

	targets := make([]string, 0, len(files))
	for _, file := range files {
		targets = append(targets, path.Join(assembly.folder, file.name))
	}

	if want := assembly.asCase().files(); !slices.Equal(sortedCopy(targets), sortedCopy(want)) {
		return fmt.Errorf("assemble %s: builds %v, the catalog lists %v", assembly.label, targets, want)
	}

	for _, target := range targets {
		if err := os.Remove(g.path(target)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("assemble %s: remove the old %s: %w", assembly.label, target, err)
		}
	}

	for index, file := range files {
		if err := g.writeAssembledFile(targets[index], file.data, assembly.modified); err != nil {
			return fmt.Errorf("assemble %s: %w", assembly.label, err)
		}
	}

	return nil
}

// writeAssembledFile writes one file and, when modified is set, its modification time.
func (g *generator) writeAssembledFile(target string, data []byte, modified time.Time) error {
	if err := writeFileAtomicallyInNewDirectory(g.path(target), data); err != nil {
		return err
	}

	if modified.IsZero() {
		return nil
	}

	if err := os.Chtimes(g.path(target), modified, modified); err != nil {
		return fmt.Errorf("set the modification time of %s: %w", target, err)
	}

	return nil
}

func sortedCopy(values []string) []string {
	sorted := slices.Clone(values)
	slices.Sort(sorted)

	return sorted
}

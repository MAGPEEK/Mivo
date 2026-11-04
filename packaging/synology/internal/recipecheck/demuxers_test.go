package recipecheck_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	// recipeScript configures FFmpeg; its DEMUXERS line is the list under test.
	recipeScript = "../../ffmpeg/build/scripts/build-ffmpeg.sh"
	// probeSource declares allowedDemuxers, the server's -format_whitelist.
	probeSource = "../../../../internal/adapters/ffmpeg/probe.go"
	// whitelistFunction is the function of probeSource that returns the whitelist.
	whitelistFunction = "allowedDemuxers"
	// imageSequenceDemuxer is added to the whitelist for paths without "%" (probe.go
	// inputDemuxers) and is therefore part of the recipe's list.
	imageSequenceDemuxer = "image2"
	// imagePipeSuffix marks the single-image demuxers, which configure names image_<x>_pipe.
	imagePipeSuffix = "_pipe"
	// imagePipePrefix is configure's prefix for those demuxers.
	imagePipePrefix = "image_"
	// programStreamProbeName and programStreamConfigureName are the two names of the MPEG
	// program stream demuxer: "mpeg" in ffprobe's output, "mpegps" in configure.
	programStreamProbeName     = "mpeg"
	programStreamConfigureName = "mpegps"
)

// demuxersAssignment matches the readonly DEMUXERS="..." line of the recipe, including its
// backslash-newline continuations.
var demuxersAssignment = regexp.MustCompile(`(?m)^readonly DEMUXERS="((?:[^"\\]|\\\n)*)"`)

// TestRecipeDemuxersMatchServerWhitelist is the static gate of the recipe's DEMUXERS line
// against the server's whitelist (ADR 0022 Notes M2-24b): both lists must name the same
// demuxers, with configure's names, so a whitelist change never silently outruns the build.
func TestRecipeDemuxersMatchServerWhitelist(t *testing.T) {
	t.Parallel()

	recipe := recipeDemuxers(t)
	whitelist := serverWhitelistAsConfigureNames(t)

	for _, name := range recipe {
		if !slices.Contains(whitelist, name) {
			t.Errorf("the recipe builds demuxer %q, which is not on the server's whitelist", name)
		}
	}

	for _, name := range whitelist {
		if !slices.Contains(recipe, name) {
			t.Errorf("the server whitelists demuxer %q, which the recipe does not build", name)
		}
	}
}

// recipeDemuxers reads the configure names of the DEMUXERS line, refusing duplicates.
func recipeDemuxers(t *testing.T) []string {
	t.Helper()

	script, err := os.ReadFile(filepath.Clean(recipeScript))
	if err != nil {
		t.Fatalf("read the recipe script: %v", err)
	}

	match := demuxersAssignment.FindSubmatch(script)
	if match == nil {
		t.Fatalf("%s has no readonly DEMUXERS=\"...\" line", recipeScript)
	}

	joined := strings.ReplaceAll(string(match[1]), "\\\n", "")
	names := strings.Split(joined, ",")

	if slices.Contains(names, "") {
		t.Fatalf("DEMUXERS of %s has an empty entry", recipeScript)
	}

	if duplicate, found := firstDuplicate(names); found {
		t.Fatalf("DEMUXERS of %s lists %q more than once", recipeScript, duplicate)
	}

	return names
}

// serverWhitelistAsConfigureNames returns allowedDemuxers of probe.go plus image2, translated
// to configure's demuxer names. Duplicates are refused after the translation, where two
// whitelist names that configure spells the same ("mpeg" and "mpegps", or an explicit "image2")
// would collapse into one.
func serverWhitelistAsConfigureNames(t *testing.T) []string {
	t.Helper()

	names := whitelistLiterals(t)
	if len(names) == 0 {
		t.Fatalf("%s: %s returns no string literal", probeSource, whitelistFunction)
	}

	configureNames := make([]string, 0, len(names)+1)

	for _, name := range names {
		configureNames = append(configureNames, configureName(name))
	}

	configureNames = append(configureNames, imageSequenceDemuxer)

	if duplicate, found := firstDuplicate(configureNames); found {
		t.Fatalf("%s: %s plus %s names configure demuxer %q more than once",
			probeSource, whitelistFunction, imageSequenceDemuxer, duplicate)
	}

	return configureNames
}

// firstDuplicate reports the first name that appears twice; a repeated entry would let the two
// lists differ in length while every name still matches.
func firstDuplicate(names []string) (string, bool) {
	seen := make(map[string]bool, len(names))

	for _, name := range names {
		if seen[name] {
			return name, true
		}

		seen[name] = true
	}

	return "", false
}

// configureName maps a demuxer name as ffprobe prints it to the name configure uses.
func configureName(probeName string) string {
	switch {
	case probeName == programStreamProbeName:
		return programStreamConfigureName
	case strings.HasSuffix(probeName, imagePipeSuffix):
		return imagePipePrefix + probeName
	default:
		return probeName
	}
}

// whitelistLiterals collects the string literals returned by allowedDemuxers in probe.go.
func whitelistLiterals(t *testing.T) []string {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), filepath.Clean(probeSource), nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", probeSource, err)
	}

	var literals []string

	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != whitelistFunction {
			continue
		}

		ast.Inspect(function.Body, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}

			value, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatalf("unquote %s: %v", literal.Value, err)
			}

			literals = append(literals, value)

			return true
		})
	}

	return literals
}

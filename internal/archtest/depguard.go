package archtest

import (
	"path"
	"regexp"
	"slices"
	"strings"
)

const (
	// golangciConfigFile is the golangci-lint configuration at the module root.
	golangciConfigFile = ".golangci.yml"
	// keyOnlyLineExpression matches a YAML mapping key without inline value, e.g. "  domain:".
	keyOnlyLineExpression = `^(\s*)([\w-]+):\s*$`
	// denyPackageLineExpression matches a depguard deny entry, e.g. "- pkg: os/exec$".
	denyPackageLineExpression = `^\s*-\s*pkg:\s*["']?([^"'\s]+)["']?\s*$`
)

// depguardDenies scans a golangci-lint configuration line by line and returns the denied
// package patterns per depguard rule. Only rules named like a layer (the base name of its
// directory, e.g. "domain") are collected. A plain line scan avoids a YAML dependency; it
// relies on the block style used in .golangci.yml.
func depguardDenies(config string, layers []layer) map[string][]string {
	ruleNames := make(map[string]bool, len(layers))
	for _, candidate := range layers {
		ruleNames[candidate.ruleName()] = true
	}

	keyOnlyLinePattern := regexp.MustCompile(keyOnlyLineExpression)
	denyPackageLinePattern := regexp.MustCompile(denyPackageLineExpression)
	denies := make(map[string][]string)
	currentRule, ruleIndent := "", 0

	for line := range strings.Lines(config) {
		if key := keyOnlyLinePattern.FindStringSubmatch(line); key != nil {
			currentRule, ruleIndent = nextRule(key, ruleNames, currentRule, ruleIndent)

			continue
		}

		if deny := denyPackageLinePattern.FindStringSubmatch(line); deny != nil && currentRule != "" {
			denies[currentRule] = append(denies[currentRule], deny[1])
		}
	}

	return denies
}

// nextRule enters a rule on its key line and leaves it on the next key at the same or a
// lower indentation.
func nextRule(key []string, ruleNames map[string]bool, currentRule string, ruleIndent int) (string, int) {
	indent, name := len(key[1]), key[2]
	if ruleNames[name] {
		return name, indent
	}

	if currentRule != "" && indent <= ruleIndent {
		return "", 0
	}

	return currentRule, ruleIndent
}

// ruleName is the depguard rule name of the layer, e.g. "domain" for "internal/domain".
func (owner layer) ruleName() string {
	return path.Base(owner.dir)
}

// missingStandardDenies returns every importable standard library package that the layer
// denies but whose exact "<package>$" deny entry is absent from configuredDenies.
func missingStandardDenies(owner layer, standardPackages, configuredDenies []string) []string {
	var missing []string

	for _, standardPackage := range standardPackages {
		if !isImportable(standardPackage) || !matchesAny(owner.denied, standardPackage) {
			continue
		}

		if !slices.Contains(configuredDenies, standardPackage+"$") {
			missing = append(missing, standardPackage)
		}
	}

	return missing
}

// isImportable reports whether code outside the standard library may import the package;
// the go command already rejects internal and vendored packages.
func isImportable(standardPackage string) bool {
	for element := range strings.SplitSeq(standardPackage, "/") {
		if element == "internal" || element == "vendor" {
			return false
		}
	}

	return true
}

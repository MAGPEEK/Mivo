package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// yamlNullTag is the resolved tag of an empty value, "~" and "null".
const yamlNullTag = "!!null"

// mappingEntrySize is the number of nodes per entry in yaml.Node.Content of a mapping:
// the key followed by its value.
const mappingEntrySize = 2

var (
	errMultipleDocuments = errors.New(`multiple YAML documents ("---") are not supported; keep a single document`)
	errNullValue         = errors.New("the key has no value (YAML null); set a value or remove the key to use the default")
	errNotScalar         = errors.New("expected a single value, not a list or mapping")
	errNotMapping        = errors.New("expected a mapping of keys")
	errDuplicateKey      = errors.New("duplicate key")
)

// fileValue is one setting found in config.yaml.
type fileValue struct {
	raw  string
	line int
}

// applyFile applies config.yaml from configDir if it exists. It never creates the file.
// Every error starts with the file path so it reads as one line in the startup message.
func applyFile(cfg *Config, configDir string) error {
	file := filepath.Join(configDir, FileName)

	data, err := os.ReadFile(filepath.Clean(file))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("read %s: %w", file, err)
	}

	values, err := parseFile(data)
	if err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}

	for _, entry := range settings() {
		value, ok := values[entry.key]
		if !ok {
			continue
		}

		if err := entry.apply(cfg, value.raw, configDir); err != nil {
			return fmt.Errorf("%s: line %d: key %s: %w", file, value.line, entry.key, err)
		}
	}

	return nil
}

// parseFile reads the single YAML document of config.yaml into values by key path. It
// walks the node tree instead of decoding into structs so that absent keys, explicit nulls
// and unknown keys are told apart and reported with the full key path and line.
func parseFile(data []byte) (map[string]fileValue, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))

	var document yaml.Node

	// io.EOF means the file has no document (empty or only comments): nothing to apply.
	err := decoder.Decode(&document)
	if errors.Is(err, io.EOF) {
		return map[string]fileValue{}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	if err := requireSingleDocument(decoder); err != nil {
		return nil, err
	}

	walker := newFileWalker()
	if err := walker.walkSection(rootOf(&document), ""); err != nil {
		return nil, err
	}

	return walker.values, nil
}

func requireSingleDocument(decoder *yaml.Decoder) error {
	var next yaml.Node

	err := decoder.Decode(&next)
	if errors.Is(err, io.EOF) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}

	return fmt.Errorf("line %d: %w", next.Line, errMultipleDocuments)
}

// fileWalker collects the settings of a YAML node tree and rejects everything else.
type fileWalker struct {
	sections map[string]bool
	leaves   map[string]bool
	seen     map[string]bool
	values   map[string]fileValue
}

// newFileWalker derives the accepted sections ("server") and leaves ("server.port") from
// the settings table, so the file accepts exactly the keys the environment accepts.
func newFileWalker() *fileWalker {
	walker := &fileWalker{
		sections: map[string]bool{},
		leaves:   map[string]bool{},
		seen:     map[string]bool{},
		values:   map[string]fileValue{},
	}

	for _, entry := range settings() {
		walker.leaves[entry.key] = true

		for _, section := range parentSections(entry.key) {
			walker.sections[section] = true
		}
	}

	return walker
}

// parentSections returns "a" and "a.b" for the key path "a.b.c".
func parentSections(key string) []string {
	parts := strings.Split(key, keySeparator)
	sections := make([]string, 0, len(parts)-1)

	for count := 1; count < len(parts); count++ {
		sections = append(sections, strings.Join(parts[:count], keySeparator))
	}

	return sections
}

// walkSection walks a mapping of keys. An empty section ("server:" without keys) and an
// empty document set nothing.
func (walker *fileWalker) walkSection(node *yaml.Node, section string) error {
	node = dereference(node)
	if isNull(node) {
		return nil
	}

	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: %s: %w: %w", node.Line, describeSection(section), ErrInvalid, errNotMapping)
	}

	for index := 0; index+1 < len(node.Content); index += mappingEntrySize {
		if err := walker.walkEntry(node.Content[index], node.Content[index+1], section); err != nil {
			return err
		}
	}

	return nil
}

func (walker *fileWalker) walkEntry(keyNode, valueNode *yaml.Node, section string) error {
	key := keyNode.Value
	if section != "" {
		key = section + keySeparator + key
	}

	switch {
	case keyNode.Kind != yaml.ScalarNode:
		return fmt.Errorf("line %d: %w: keys must be plain names", keyNode.Line, ErrUnknownKey)
	case walker.seen[key]:
		return fmt.Errorf("line %d: %w %q", keyNode.Line, errDuplicateKey, key)
	case walker.sections[key]:
		walker.seen[key] = true

		return walker.walkSection(valueNode, key)
	case walker.leaves[key]:
		walker.seen[key] = true

		return walker.addValue(key, keyNode.Line, valueNode)
	default:
		return fmt.Errorf("line %d: %w %q", keyNode.Line, ErrUnknownKey, key)
	}
}

func (walker *fileWalker) addValue(key string, line int, node *yaml.Node) error {
	node = dereference(node)

	switch {
	case isNull(node):
		return fmt.Errorf("line %d: key %s: %w: %w", line, key, ErrInvalid, errNullValue)
	case node.Kind != yaml.ScalarNode:
		return fmt.Errorf("line %d: key %s: %w: %w", line, key, ErrInvalid, errNotScalar)
	default:
		walker.values[key] = fileValue{raw: node.Value, line: line}

		return nil
	}
}

func describeSection(section string) string {
	if section == "" {
		return "top level"
	}

	return "section " + section
}

// dereference follows an alias ("*name") to its anchored node.
func dereference(node *yaml.Node) *yaml.Node {
	if node.Kind == yaml.AliasNode && node.Alias != nil {
		return node.Alias
	}

	return node
}

func isNull(node *yaml.Node) bool {
	return node.Kind == yaml.ScalarNode && node.ShortTag() == yamlNullTag
}

// rootOf returns the top-level node of a decoded document; a document without content
// counts as empty.
func rootOf(document *yaml.Node) *yaml.Node {
	if len(document.Content) == 0 {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: yamlNullTag}
	}

	return document.Content[0]
}

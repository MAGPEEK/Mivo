package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
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
	errNotList           = errors.New("expected a list of entries")
	errDuplicateKey      = errors.New("duplicate key")
)

// fileValue is one scalar value found in config.yaml.
type fileValue struct {
	raw  string
	line int
}

// fileItem is one entry of a list setting, e.g. one library: its fields by name.
type fileItem struct {
	line   int
	fields map[string]fileValue
}

// fileContent is everything config.yaml sets: scalar settings by key path and list
// settings by key.
type fileContent struct {
	values map[string]fileValue
	lists  map[string][]fileItem
}

// applyFile applies config.yaml from configDir if it exists and returns the libraries it
// defines (nil when it defines none). It never creates the file. Every error starts with
// the file path so it reads as one line in the startup message.
func applyFile(cfg *Config, configDir string) ([]configuredLibrary, error) {
	file := filepath.Join(configDir, FileName)

	data, err := os.ReadFile(filepath.Clean(file))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file, err)
	}

	content, err := parseFile(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}

	for _, entry := range settings() {
		value, ok := content.values[entry.key]
		if !ok {
			continue
		}

		if err := entry.apply(cfg, value.raw, configDir); err != nil {
			return nil, fmt.Errorf("%s: line %d: key %s: %w", file, value.line, entry.key, err)
		}
	}

	return fileLibraries(content.lists[keyLibraries], file, configDir)
}

// parseFile reads the single YAML document of config.yaml. It walks the node tree instead
// of decoding into structs so that absent keys, explicit nulls and unknown keys are told
// apart and reported with the full key path and line.
func parseFile(data []byte) (fileContent, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	walker := newFileWalker()

	var document yaml.Node

	// io.EOF means the file has no document (empty or only comments): nothing to apply.
	err := decoder.Decode(&document)
	if errors.Is(err, io.EOF) {
		return walker.content, nil
	}

	if err != nil {
		return fileContent{}, fmt.Errorf("%w: invalid YAML: %w", ErrInvalid, err)
	}

	if err := requireSingleDocument(decoder); err != nil {
		return fileContent{}, err
	}

	if err := walker.walkSection(rootOf(&document), ""); err != nil {
		return fileContent{}, err
	}

	return walker.content, nil
}

func requireSingleDocument(decoder *yaml.Decoder) error {
	var next yaml.Node

	err := decoder.Decode(&next)
	if errors.Is(err, io.EOF) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("%w: invalid YAML: %w", ErrInvalid, err)
	}

	return fmt.Errorf("line %d: %w: %w", next.Line, ErrInvalid, errMultipleDocuments)
}

// fileWalker collects the settings of a YAML node tree and rejects everything else.
type fileWalker struct {
	sections map[string]bool
	leaves   map[string]bool
	// lists maps the key of a list setting to the field names its entries accept.
	lists   map[string][]string
	seen    map[string]bool
	content fileContent
}

// newFileWalker derives the accepted sections ("server"), leaves ("server.port") and lists
// ("libraries") from the settings tables, so the file accepts exactly the keys the
// environment accepts.
func newFileWalker() *fileWalker {
	walker := &fileWalker{
		sections: map[string]bool{},
		leaves:   map[string]bool{},
		lists:    map[string][]string{keyLibraries: libraryFieldNames()},
		seen:     map[string]bool{},
		content:  fileContent{values: map[string]fileValue{}, lists: map[string][]fileItem{}},
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
	if keyNode.Kind != yaml.ScalarNode {
		return fmt.Errorf("line %d: %w %s: keys must be plain names", keyNode.Line, ErrUnknownKey, inSection(section))
	}

	key := joinKey(section, keyNode.Value)
	if walker.seen[key] {
		return fmt.Errorf("line %d: key %s: %w: %w", keyNode.Line, key, ErrInvalid, errDuplicateKey)
	}

	walker.seen[key] = true

	switch {
	case walker.sections[key]:
		return walker.walkSection(valueNode, key)
	case walker.leaves[key]:
		return walker.addValue(key, keyNode.Line, valueNode)
	case walker.lists[key] != nil:
		return walker.walkList(key, keyNode.Line, valueNode)
	default:
		return fmt.Errorf("line %d: %w %q", keyNode.Line, ErrUnknownKey, key)
	}
}

func (walker *fileWalker) addValue(key string, line int, node *yaml.Node) error {
	value, err := scalarValue(key, line, node)
	if err != nil {
		return err
	}

	walker.content.values[key] = value

	return nil
}

// walkList walks a list of mappings such as libraries. An empty list ("[]") is valid and
// sets an empty list; a null value is rejected like any other null.
func (walker *fileWalker) walkList(key string, line int, node *yaml.Node) error {
	node = dereference(node)

	switch {
	case isNull(node):
		return fmt.Errorf("line %d: key %s: %w: %w", line, key, ErrInvalid, errNullValue)
	case node.Kind != yaml.SequenceNode:
		return fmt.Errorf("line %d: key %s: %w: %w", node.Line, key, ErrInvalid, errNotList)
	}

	items := make([]fileItem, 0, len(node.Content))

	for index, itemNode := range node.Content {
		item, err := walkListItem(listItemLabel(key, index), dereference(itemNode), walker.lists[key])
		if err != nil {
			return err
		}

		items = append(items, item)
	}

	walker.content.lists[key] = items

	return nil
}

// walkListItem reads one entry of a list: a mapping of the allowed fields to single values.
func walkListItem(label string, node *yaml.Node, fields []string) (fileItem, error) {
	if node.Kind != yaml.MappingNode {
		return fileItem{}, fmt.Errorf("line %d: key %s: %w: %w", node.Line, label, ErrInvalid, errNotMapping)
	}

	item := fileItem{line: node.Line, fields: map[string]fileValue{}}

	for index := 0; index+1 < len(node.Content); index += mappingEntrySize {
		keyNode, valueNode := node.Content[index], node.Content[index+1]
		if err := checkListItemKey(label, keyNode, fields, item); err != nil {
			return fileItem{}, err
		}

		value, err := scalarValue(joinKey(label, keyNode.Value), keyNode.Line, valueNode)
		if err != nil {
			return fileItem{}, err
		}

		item.fields[keyNode.Value] = value
	}

	return item, nil
}

// checkListItemKey accepts each allowed field of a list entry once.
func checkListItemKey(label string, keyNode *yaml.Node, fields []string, item fileItem) error {
	key := joinKey(label, keyNode.Value)
	_, seen := item.fields[keyNode.Value]

	switch {
	case keyNode.Kind != yaml.ScalarNode:
		return fmt.Errorf("line %d: %w in %s: keys must be plain names", keyNode.Line, ErrUnknownKey, label)
	case !slices.Contains(fields, keyNode.Value):
		return fmt.Errorf("line %d: %w %q", keyNode.Line, ErrUnknownKey, key)
	case seen:
		return fmt.Errorf("line %d: key %s: %w: %w", keyNode.Line, key, ErrInvalid, errDuplicateKey)
	default:
		return nil
	}
}

// scalarValue returns the single value of key; nulls, lists and mappings are rejected.
func scalarValue(key string, line int, node *yaml.Node) (fileValue, error) {
	node = dereference(node)

	switch {
	case isNull(node):
		return fileValue{}, fmt.Errorf("line %d: key %s: %w: %w", line, key, ErrInvalid, errNullValue)
	case node.Kind != yaml.ScalarNode:
		return fileValue{}, fmt.Errorf("line %d: key %s: %w: %w", line, key, ErrInvalid, errNotScalar)
	default:
		return fileValue{raw: node.Value, line: line}, nil
	}
}

func joinKey(section, key string) string {
	if section == "" {
		return key
	}

	return section + keySeparator + key
}

// listItemLabel names an entry of a list setting the way errors show it, e.g. "libraries[1]".
func listItemLabel(key string, index int) string {
	return fmt.Sprintf("%s[%d]", key, index)
}

func describeSection(section string) string {
	if section == "" {
		return "top level"
	}

	return "section " + section
}

func inSection(section string) string {
	if section == "" {
		return "at the top level"
	}

	return "in section " + section
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

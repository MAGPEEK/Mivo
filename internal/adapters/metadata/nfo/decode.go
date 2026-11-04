package nfo

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"

	"golang.org/x/text/encoding/charmap"
)

// Declaration parsing. maxDeclarationBytes bounds the search for the end of the XML
// declaration so a missing "?>" does not scan the whole file.
const (
	declarationStart    = "<?xml"
	declarationEnd      = "?>"
	encodingAttribute   = "encoding="
	maxDeclarationBytes = 256
)

// node is one element of the decoded tree. Names are lowercase; text is the concatenated
// character data directly inside the element.
type node struct {
	name       string
	attributes []xml.Attr
	text       strings.Builder
	children   []*node
}

// attribute returns the value of the attribute with the given lowercase local name.
func (element *node) attribute(name string) string {
	for _, attribute := range element.attributes {
		if strings.EqualFold(attribute.Name.Local, name) {
			return attribute.Value
		}
	}

	return ""
}

// document is the decoded file: its top-level elements and the text after the last one,
// where Kodi allows a provider URL.
type document struct {
	roots        []*node
	trailingText string
}

func (doc document) rootsNamed(name string) []*node {
	var matching []*node

	for _, root := range doc.roots {
		if root.name == name {
			matching = append(matching, root)
		}
	}

	return matching
}

// toUTF8 converts the file to UTF-8 according to its XML declaration. Only UTF-8 and the
// two Latin encodings Kodi tools write are accepted (ADR 0027 §7).
func toUTF8(data []byte) (string, error) {
	label := declaredEncoding(data)

	switch {
	case label == "" || isUTF8Label(label):
		return string(data), nil
	case isLatin1Label(label):
		return decodeWith(charmap.ISO8859_1, data)
	case isWindows1252Label(label):
		return decodeWith(charmap.Windows1252, data)
	default:
		return "", unsupported("unsupported encoding")
	}
}

func decodeWith(encoding *charmap.Charmap, data []byte) (string, error) {
	converted, err := encoding.NewDecoder().Bytes(data)
	if err != nil {
		return "", unsupported("undecodable text")
	}

	return string(converted), nil
}

// declaredEncoding returns the lowercase encoding label of the XML declaration, or "".
func declaredEncoding(data []byte) string {
	if !bytes.HasPrefix(data, []byte(declarationStart)) {
		return ""
	}

	window := data[:min(len(data), maxDeclarationBytes)]

	end := bytes.Index(window, []byte(declarationEnd))
	if end < 0 {
		return ""
	}

	declaration := string(window[:end])

	_, afterName, found := strings.Cut(declaration, encodingAttribute)
	if !found || len(afterName) < 2 {
		return ""
	}

	quote := afterName[0]
	if quote != '"' && quote != '\'' {
		return ""
	}

	label, _, closed := strings.Cut(afterName[1:], string(quote))
	if !closed {
		return ""
	}

	return strings.ToLower(strings.TrimSpace(label))
}

func isUTF8Label(label string) bool {
	return label == "utf-8" || label == "utf8"
}

func isLatin1Label(label string) bool {
	switch label {
	case "iso-8859-1", "iso8859-1", "iso_8859-1", "latin1", "latin-1", "l1":
		return true
	default:
		return false
	}
}

func isWindows1252Label(label string) bool {
	return label == "windows-1252" || label == "cp1252"
}

// passThroughCharset satisfies encoding/xml for the Latin labels: toUTF8 already converted
// the text, so the declaration no longer describes the bytes.
func passThroughCharset(label string, input io.Reader) (io.Reader, error) {
	lower := strings.ToLower(label)
	if isLatin1Label(lower) || isWindows1252Label(lower) {
		return input, nil
	}

	return nil, errors.New("unsupported encoding")
}

// treeBuilder turns the decoder's token stream into nodes while enforcing the limits.
type treeBuilder struct {
	doc          document
	open         []*node
	elementCount int
	lastRootEnd  int64
}

// decodeDocument parses the UTF-8 text. encoding/xml never resolves external entities or
// DTDs; HTML entities such as &nbsp; are decoded.
func decodeDocument(text string) (document, error) {
	decoder := xml.NewDecoder(strings.NewReader(text))
	decoder.Strict = true
	decoder.Entity = xml.HTMLEntity
	decoder.CharsetReader = passThroughCharset

	var builder treeBuilder

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return builder.finishAfterError(text, err)
		}

		if err := builder.add(token, decoder.InputOffset()); err != nil {
			return document{}, err
		}
	}

	if len(builder.open) > 0 {
		return document{}, unsupported("malformed XML: unclosed element")
	}

	builder.doc.trailingText = text[builder.lastRootEnd:]

	return builder.doc, nil
}

// finishAfterError accepts a syntax error only in plain text after the last complete root,
// which is where Kodi puts a provider URL whose "&" is no valid XML.
func (builder *treeBuilder) finishAfterError(text string, err error) (document, error) {
	remainder := text[builder.lastRootEnd:]
	if len(builder.open) > 0 || len(builder.doc.roots) == 0 || strings.Contains(remainder, "<") {
		return document{}, malformed(err)
	}

	builder.doc.trailingText = remainder

	return builder.doc, nil
}

func malformed(err error) error {
	var syntaxError *xml.SyntaxError
	if errors.As(err, &syntaxError) {
		return unsupported("malformed XML at line %d", syntaxError.Line)
	}

	return unsupported("malformed XML")
}

func (builder *treeBuilder) add(token xml.Token, offset int64) error {
	switch typed := token.(type) {
	case xml.StartElement:
		return builder.start(typed)
	case xml.EndElement:
		builder.end(offset)
	case xml.CharData:
		if len(builder.open) > 0 {
			builder.open[len(builder.open)-1].text.Write(typed)
		}
	}

	return nil
}

func (builder *treeBuilder) start(element xml.StartElement) error {
	builder.elementCount++
	if builder.elementCount > maxElementCount {
		return unsupported("more than %d elements", maxElementCount)
	}

	if len(builder.open) >= maxElementDepth {
		return unsupported("elements nested deeper than %d", maxElementDepth)
	}

	child := &node{name: strings.ToLower(element.Name.Local), attributes: element.Attr}

	if len(builder.open) == 0 {
		builder.doc.roots = append(builder.doc.roots, child)
	} else {
		parent := builder.open[len(builder.open)-1]
		parent.children = append(parent.children, child)
	}

	builder.open = append(builder.open, child)

	return nil
}

func (builder *treeBuilder) end(offset int64) {
	// encoding/xml guarantees matching end elements in strict mode, so open is never empty.
	builder.open = builder.open[:len(builder.open)-1]
	if len(builder.open) == 0 {
		builder.lastRootEnd = offset
	}
}

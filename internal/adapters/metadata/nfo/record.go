package nfo

import (
	"slices"
	"strconv"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/magpeek/mivo/internal/domain/metadata"
)

// Year bounds of the <year> element; anything outside is a typo, not a release year.
const (
	minimumYear = 1
	maximumYear = 9999
	// dateLength is the length of the "YYYY-MM-DD" prefix of a date value; some tools append
	// a time of day.
	dateLength = len("2006-01-02")
)

// recordBuilder maps one root element to a record and collects the warnings.
type recordBuilder struct {
	root     *node
	kind     Kind
	record   metadata.Record
	warnings []string
}

func buildRecord(root *node, kind Kind) (metadata.Record, []string) {
	builder := recordBuilder{root: root, kind: kind}
	builder.readTexts()
	builder.readDates()
	builder.readLists()
	builder.readPeople()
	builder.record.ProviderIDs = builder.readProviderIDs()

	return builder.record, builder.warnings
}

func (builder *recordBuilder) warn(code string) {
	if !slices.Contains(builder.warnings, code) {
		builder.warnings = append(builder.warnings, code)
	}
}

func (builder *recordBuilder) readTexts() {
	record := &builder.record
	record.Title = builder.firstText("title")
	record.OriginalTitle = builder.firstText("originaltitle")
	record.SortTitle = builder.firstText("sorttitle")
	record.Summary = builder.firstText("plot", "outline")
	record.Tagline = builder.firstText("tagline")
	record.ContentRating = builder.firstText("mpaa", "certification")
}

// readDates reads the release date and the year; the year falls back to the date's year.
func (builder *recordBuilder) readDates() {
	for _, name := range []string{"premiered", "releasedate", "aired"} {
		if date, ok := builder.date(name); ok {
			builder.record.ReleaseDate = date

			break
		}
	}

	if text := builder.firstText("year"); text != "" {
		year, err := strconv.Atoi(text)
		if err == nil && year >= minimumYear && year <= maximumYear {
			builder.record.Year = year
		} else {
			builder.warn(WarningInvalidYear)
		}
	}

	if builder.record.Year == 0 {
		builder.record.Year = builder.record.ReleaseDate.Year()
	}
}

func (builder *recordBuilder) date(name string) (metadata.Date, bool) {
	text := builder.firstText(name)
	if text == "" {
		return metadata.Date{}, false
	}

	if len(text) > dateLength && (text[dateLength] == ' ' || text[dateLength] == 'T') {
		text = text[:dateLength]
	}

	date, err := metadata.ParseDate(text)
	if err != nil {
		builder.warn(WarningInvalidDate)

		return metadata.Date{}, false
	}

	return date, true
}

func (builder *recordBuilder) readLists() {
	record := &builder.record
	record.Genres = builder.allTexts("genre")
	record.Studios = builder.allTexts("studio")
	record.Countries = builder.allTexts("country")
	record.Collections = builder.collections()
}

// collections reads Kodi's <set><name>…</name></set> and the older plain-text <set>.
func (builder *recordBuilder) collections() []string {
	var names []string

	for _, set := range builder.children("set") {
		name := cleanText(set.text.String())
		if nested := firstChild(set, "name"); nested != nil {
			name = cleanText(nested.text.String())
		}

		names = appendUnique(names, name)
	}

	return names
}

// readPeople lists directors, writers and actors in that order; actors keep their <order>.
func (builder *recordBuilder) readPeople() {
	var people []metadata.Credit

	people = appendCredits(people, builder.allTexts("director"), metadata.RoleDirector)
	people = appendCredits(people, builder.allTexts("credits", "writer"), metadata.RoleWriter)

	for position, actor := range builder.children("actor") {
		credit, ok := builder.actorCredit(actor, position)
		if ok && !containsCredit(people, credit) {
			people = append(people, credit)
		}
	}

	builder.record.People = people
}

func (builder *recordBuilder) actorCredit(actor *node, position int) (metadata.Credit, bool) {
	name := childText(actor, "name")
	if name == "" {
		return metadata.Credit{}, false
	}

	credit := metadata.Credit{Name: name, Role: metadata.RoleActor, Character: childText(actor, "role"), Order: position}

	if orderText := childText(actor, "order"); orderText != "" {
		order, err := strconv.Atoi(orderText)
		if err == nil && order >= 0 {
			credit.Order = order
		} else {
			builder.warn(WarningInvalidOrder)
		}
	}

	return credit, true
}

func appendCredits(people []metadata.Credit, names []string, role metadata.Role) []metadata.Credit {
	order := 0

	for _, name := range names {
		credit := metadata.Credit{Name: name, Role: role, Order: order}
		if !containsCredit(people, credit) {
			people = append(people, credit)
			order++
		}
	}

	return people
}

func containsCredit(people []metadata.Credit, credit metadata.Credit) bool {
	return slices.ContainsFunc(people, func(existing metadata.Credit) bool {
		return existing.Name == credit.Name && existing.Role == credit.Role
	})
}

// children returns the direct children with one of the names, in document order.
func (builder *recordBuilder) children(names ...string) []*node {
	var matching []*node

	for _, child := range builder.root.children {
		if slices.Contains(names, child.name) {
			matching = append(matching, child)
		}
	}

	return matching
}

// firstText returns the first non-empty text of the names, tried in the given order.
func (builder *recordBuilder) firstText(names ...string) string {
	for _, name := range names {
		for _, child := range builder.children(name) {
			if text := cleanText(child.text.String()); text != "" {
				return text
			}
		}
	}

	return ""
}

// allTexts returns the distinct non-empty texts of the elements, in document order.
func (builder *recordBuilder) allTexts(names ...string) []string {
	var texts []string

	for _, child := range builder.children(names...) {
		texts = appendUnique(texts, cleanText(child.text.String()))
	}

	return texts
}

func appendUnique(texts []string, text string) []string {
	if text == "" || slices.Contains(texts, text) {
		return texts
	}

	return append(texts, text)
}

func firstChild(parent *node, name string) *node {
	for _, child := range parent.children {
		if child.name == name {
			return child
		}
	}

	return nil
}

func childText(parent *node, name string) string {
	if child := firstChild(parent, name); child != nil {
		return cleanText(child.text.String())
	}

	return ""
}

// cleanText trims the text and normalises it to NFC (ADR 0027): macOS tools write NFD, and
// equal names must compare equal in the catalog. Inner whitespace and line breaks stay.
func cleanText(text string) string {
	return norm.NFC.String(strings.TrimSpace(text))
}

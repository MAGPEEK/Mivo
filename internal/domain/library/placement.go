package library

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
)

// Separators of the canonical placement key. Match keys and the version label are quoted
// with strconv.Quote, so these characters can never be confused with key content.
const (
	keyLevelSeparator = "/"
	keyFieldSeparator = ";"
	keyKindSeparator  = " "
	keyVersionField   = "version "
	keyPartField      = "part"
)

// Level is one item of a placement chain: its kind, its match key among its siblings
// (ADR 0027 §3), the title the parser derived and the numbers it found. Numbers are only set
// where they apply: Year for movies and shows, Number for seasons and episodes, NumberEnd for
// multi-episode files, AbsoluteNumber for absolute-numbered episodes; ExtraType for extras.
type Level struct {
	Kind           ItemKind
	MatchKey       string
	Title          string
	Year           OptionalNumber
	Number         OptionalNumber
	NumberEnd      OptionalNumber
	AbsoluteNumber OptionalNumber
	ExtraType      ExtraType
}

// Placement says where one video file belongs in a library's item hierarchy: the chain of
// levels from the top-level item (first) to the item that owns the file (last), plus the
// file's version label and part number within that item. The name parser computes it from
// the path on every scan; Key makes stored and computed placements comparable.
type Placement struct {
	Levels       []Level
	VersionLabel string
	PartNumber   OptionalNumber
}

// Validate checks the placement against the hierarchy of ADR 0027 §2: at least one level,
// every level allowed below the one before it (the first at the top level), a leaf that owns
// files, non-blank match keys and titles, non-negative numbers, a multi-episode end not
// before its start, and an extra type exactly on extras. The error wraps domain.ErrInvalid.
func (placement Placement) Validate() error {
	if len(placement.Levels) == 0 {
		return fmt.Errorf("validate placement: no level: %w", domain.ErrInvalid)
	}

	parent := ItemKind(noParent)

	for index, level := range placement.Levels {
		if err := level.validate(parent); err != nil {
			return fmt.Errorf("validate placement level %d: %w", index, err)
		}

		parent = level.Kind
	}

	if !parent.OwnsFiles() {
		return fmt.Errorf("validate placement: leaf %q owns no files: %w", parent, domain.ErrInvalid)
	}

	if err := checkNonNegative("part number", placement.PartNumber); err != nil {
		return fmt.Errorf("validate placement: %w", err)
	}

	return nil
}

// Key returns the canonical text of the placement's identity: the kinds and match keys of
// the chain, the version label and the part number. Titles, years and other numbers are
// display data and do not take part, so a retitled file keeps its placement.
func (placement Placement) Key() string {
	levels := make([]string, 0, len(placement.Levels))
	for _, level := range placement.Levels {
		levels = append(levels, level.Kind.String()+keyKindSeparator+strconv.Quote(level.MatchKey))
	}

	part := keyPartField
	if number, ok := placement.PartNumber.Get(); ok {
		part += keyKindSeparator + strconv.Itoa(number)
	}

	return strings.Join(levels, keyLevelSeparator) +
		keyFieldSeparator + keyVersionField + strconv.Quote(placement.VersionLabel) +
		keyFieldSeparator + part
}

func (level Level) validate(parent ItemKind) error {
	if !ValidParent(level.Kind, parent) {
		return fmt.Errorf("kind %q not allowed below %q: %w", level.Kind, parent, domain.ErrInvalid)
	}

	if strings.TrimSpace(level.MatchKey) == "" {
		return fmt.Errorf("%s has an empty match key: %w", level.Kind, domain.ErrInvalid)
	}

	if strings.TrimSpace(level.Title) == "" {
		return fmt.Errorf("%s %q has an empty title: %w", level.Kind, level.MatchKey, domain.ErrInvalid)
	}

	if err := level.validateNumbers(); err != nil {
		return fmt.Errorf("%s %q: %w", level.Kind, level.MatchKey, err)
	}

	if err := level.validateExtraType(); err != nil {
		return fmt.Errorf("%s %q: %w", level.Kind, level.MatchKey, err)
	}

	return nil
}

func (level Level) validateNumbers() error {
	err := errors.Join(
		checkNonNegative("year", level.Year),
		checkNonNegative("number", level.Number),
		checkNonNegative("number end", level.NumberEnd),
		checkNonNegative("absolute number", level.AbsoluteNumber),
	)
	if err != nil {
		return err
	}

	end, hasEnd := level.NumberEnd.Get()
	if !hasEnd {
		return nil
	}

	start, hasStart := level.Number.Get()
	if !hasStart {
		return fmt.Errorf("number end %d without a number: %w", end, domain.ErrInvalid)
	}

	if end < start {
		return fmt.Errorf("number end %d before number %d: %w", end, start, domain.ErrInvalid)
	}

	return nil
}

func (level Level) validateExtraType() error {
	if level.Kind != ItemKindExtra {
		if level.ExtraType != "" {
			return fmt.Errorf("extra type %q on a non-extra: %w", level.ExtraType, domain.ErrInvalid)
		}

		return nil
	}

	if !level.ExtraType.IsValid() {
		return fmt.Errorf("unknown extra type %q: %w", level.ExtraType, domain.ErrInvalid)
	}

	return nil
}

func checkNonNegative(name string, number OptionalNumber) error {
	if value, ok := number.Get(); ok && value < 0 {
		return fmt.Errorf("%s %d is negative: %w", name, value, domain.ErrInvalid)
	}

	return nil
}

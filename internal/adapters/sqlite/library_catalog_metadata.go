package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

var _ scan.MetadataCatalog = (*LibraryCatalog)(nil)

// characterSeparator joins the characters one actor plays in one item: item_people has one
// row per (item, person, role).
const characterSeparator = " / "

// ItemMetadataState returns the stored record, field locks and artwork selection of an item;
// an unknown item wraps domain.ErrNotFound.
func (catalog *LibraryCatalog) ItemMetadataState(ctx context.Context, itemID string) (scan.MetadataState, error) {
	var state scan.MetadataState

	err := inReadTransaction(ctx, catalog.reader, func(queries *sqlcgen.Queries) error {
		var err error

		state, err = readMetadataState(ctx, queries, itemID)

		return err
	})
	if err != nil {
		return scan.MetadataState{}, fmt.Errorf("metadata state of item %q: %w", itemID, err)
	}

	return state, nil
}

func readMetadataState(ctx context.Context, queries *sqlcgen.Queries, itemID string) (scan.MetadataState, error) {
	record, err := readStoredRecord(ctx, queries, itemID)
	if err != nil {
		return scan.MetadataState{}, err
	}

	locks, err := readFieldLocks(ctx, queries, itemID)
	if err != nil {
		return scan.MetadataState{}, err
	}

	locked, err := lockedArtworkKinds(locks)
	if err != nil {
		return scan.MetadataState{}, err
	}

	state := scan.MetadataState{
		Record: record, Locks: locks, LockedArtwork: locked,
		CurrentArtwork: map[artwork.Kind]artwork.Candidate{},
	}

	return state, readSelectedArtwork(ctx, queries, itemID, &state)
}

// lockedArtworkKinds maps the artwork.* locks to the kinds.
func lockedArtworkKinds(locks metadata.LockSet) (map[artwork.Kind]bool, error) {
	locked := map[artwork.Kind]bool{}

	for _, kind := range artwork.Kinds() {
		field, err := metadata.ArtworkField(kind)
		if err != nil {
			return nil, err
		}

		locked[kind] = locks.Has(field)
	}

	return locked, nil
}

func readStoredRecord(ctx context.Context, queries *sqlcgen.Queries, itemID string) (metadata.Record, error) {
	row, err := queries.GetItem(ctx, itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return metadata.Record{}, domain.ErrNotFound
	}

	if err != nil {
		return metadata.Record{}, fmt.Errorf("read item: %w", err)
	}

	var detail applibrary.ItemDetail
	if err := readDetailMetadata(ctx, queries, row, &detail); err != nil {
		return metadata.Record{}, err
	}

	return detail.Metadata, nil
}

func readFieldLocks(ctx context.Context, queries *sqlcgen.Queries, itemID string) (metadata.LockSet, error) {
	names, err := queries.ListItemFieldLocks(ctx, itemID)
	if err != nil {
		return metadata.LockSet{}, fmt.Errorf("read field locks: %w", err)
	}

	fields := make([]metadata.Field, 0, len(names))

	for _, name := range names {
		field, err := metadata.ParseField(name)
		if err != nil {
			return metadata.LockSet{}, undecodable(fmt.Sprintf("item %q: field lock", itemID), err)
		}

		fields = append(fields, field)
	}

	return metadata.NewLockSet(fields), nil
}

// SaveMetadata replaces the merged record of an item. A locked field keeps its stored value
// even when record differs (the integrity guard of ADR 0027 section 6). Tags, credits and
// provider IDs are replaced in one transaction and tags and people no item uses any more
// are deleted. Errors wrap domain.ErrNotFound for an unknown item and domain.ErrInvalid for
// an invalid record.
func (catalog *LibraryCatalog) SaveMetadata(ctx context.Context, itemID string, record metadata.Record) error {
	err := inWriteTransaction(ctx, catalog.writer, func(queries *sqlcgen.Queries) error {
		return saveMetadata(ctx, queries, itemID, record)
	})
	if err != nil {
		return fmt.Errorf("save metadata of item %q: %w", itemID, err)
	}

	return nil
}

func saveMetadata(ctx context.Context, queries *sqlcgen.Queries, itemID string, record metadata.Record) error {
	state, err := readMetadataState(ctx, queries, itemID)
	if err != nil {
		return err
	}

	merged := metadata.Merge(state.Record, state.Locks, []metadata.SourcedRecord{
		{Source: metadata.SourceUser, Record: record},
	})
	if err := validateRecord(merged); err != nil {
		return err
	}

	if err := updateItemColumns(ctx, queries, itemID, merged); err != nil {
		return err
	}

	writer := metadataWriter{queries: queries, itemID: itemID}

	return writer.replaceRelations(ctx, merged)
}

// validateRecord rejects what would otherwise surface as a constraint error.
func validateRecord(record metadata.Record) error {
	if strings.TrimSpace(record.Title) == "" {
		return fmt.Errorf("record has no title: %w", domain.ErrInvalid)
	}

	if record.Year < 0 {
		return fmt.Errorf("record has year %d: %w", record.Year, domain.ErrInvalid)
	}

	for _, credit := range record.People {
		if !credit.Role.IsValid() || strings.TrimSpace(credit.Name) == "" || credit.Order < 0 {
			return fmt.Errorf("credit %q (role %q, order %d): %w", credit.Name, credit.Role, credit.Order, domain.ErrInvalid)
		}
	}

	for _, id := range record.ProviderIDs {
		if err := metadata.ValidateProviderID(id.Provider, id.Value); err != nil {
			return err
		}
	}

	return nil
}

func updateItemColumns(ctx context.Context, queries *sqlcgen.Queries, itemID string, record metadata.Record) error {
	updated, err := queries.UpdateItemMetadata(ctx, sqlcgen.UpdateItemMetadataParams{
		Title: record.Title, SortTitle: metadata.SortTitleFor(record), OriginalTitle: record.OriginalTitle,
		Year: int64(record.Year), ReleaseDate: record.ReleaseDate.String(), Summary: record.Summary,
		Tagline: record.Tagline, ContentRating: record.ContentRating,
		UpdatedAt: time.Now().UnixMilli(), ID: itemID,
	})
	if err != nil {
		return fmt.Errorf("update item: %w", err)
	}

	if updated == 0 {
		return domain.ErrNotFound
	}

	return storeAgeRating(ctx, queries, itemID, record.ContentRating)
}

// metadataWriter replaces the tags, credits and provider IDs of one item.
type metadataWriter struct {
	queries *sqlcgen.Queries
	itemID  string
}

func (writer metadataWriter) replaceRelations(ctx context.Context, record metadata.Record) error {
	for name, remove := range map[string]func(context.Context, string) error{
		"tags": writer.queries.DeleteItemTags, "people": writer.queries.DeleteItemPeople,
		"provider IDs": writer.queries.DeleteItemProviderIDs,
	} {
		if err := remove(ctx, writer.itemID); err != nil {
			return fmt.Errorf("delete %s: %w", name, err)
		}
	}

	if err := writer.insertTags(ctx, record); err != nil {
		return err
	}

	if err := writer.insertCredits(ctx, record.People); err != nil {
		return err
	}

	for _, id := range record.ProviderIDs {
		err := writer.queries.InsertItemProviderID(ctx, sqlcgen.InsertItemProviderIDParams{
			ItemID: writer.itemID, Provider: id.Provider.String(), ExternalID: id.Value,
		})
		if err != nil {
			return fmt.Errorf("insert provider ID: %w", uniqueAsConflict(err))
		}
	}

	return writer.deleteUnused(ctx)
}

func (writer metadataWriter) deleteUnused(ctx context.Context) error {
	if err := writer.queries.DeleteUnusedTags(ctx); err != nil {
		return fmt.Errorf("delete unused tags: %w", err)
	}

	if err := writer.queries.DeleteUnusedPeople(ctx); err != nil {
		return fmt.Errorf("delete unused people: %w", err)
	}

	return nil
}

func (writer metadataWriter) insertTags(ctx context.Context, record metadata.Record) error {
	groups := []struct {
		kind  string
		names []string
	}{
		{tagKindGenre, record.Genres},
		{tagKindStudio, record.Studios},
		{tagKindCountry, record.Countries},
		{tagKindCollection, record.Collections},
	}

	for _, group := range groups {
		position := 0

		for _, name := range uniqueNames(group.names) {
			tagID, err := writer.tagID(ctx, group.kind, name)
			if err != nil {
				return err
			}

			err = writer.queries.InsertItemTag(ctx, sqlcgen.InsertItemTagParams{
				ItemID: writer.itemID, TagID: tagID, Position: int64(position),
			})
			if err != nil {
				return fmt.Errorf("insert item tag: %w", uniqueAsConflict(err))
			}

			position++
		}
	}

	return nil
}

// uniqueNames trims the names and drops empty ones and case-insensitive repeats; the first
// spelling wins.
func uniqueNames(names []string) []string {
	unique := make([]string, 0, len(names))
	seen := map[string]bool{}

	for _, name := range names {
		name = strings.TrimSpace(name)
		folded := strings.ToLower(name)

		if name == "" || seen[folded] {
			continue
		}

		seen[folded] = true

		unique = append(unique, name)
	}

	return unique
}

func (writer metadataWriter) tagID(ctx context.Context, kind, name string) (string, error) {
	id, err := writer.queries.FindTagID(ctx, sqlcgen.FindTagIDParams{Kind: kind, Name: name})
	if err == nil {
		return id, nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("find tag: %w", err)
	}

	id = uuid.NewV4().String()
	if err := writer.queries.InsertTag(ctx, sqlcgen.InsertTagParams{ID: id, Kind: kind, Name: name}); err != nil {
		return "", fmt.Errorf("insert tag: %w", uniqueAsConflict(err))
	}

	return id, nil
}

// creditGroup is all credits of one person in one role.
type creditGroup struct {
	name       string
	role       metadata.Role
	characters []string
	order      int
}

// groupCredits merges credits of the same person (case-insensitive) and role, because
// item_people holds one row per (item, person, role): the characters are joined in credit
// order, the group takes the position of its first credit.
func groupCredits(credits []metadata.Credit) []creditGroup {
	ordered := slices.Clone(credits)
	slices.SortStableFunc(ordered, func(first, second metadata.Credit) int { return first.Order - second.Order })

	var groups []creditGroup

	index := map[string]int{}

	for _, credit := range ordered {
		name := strings.TrimSpace(credit.Name)
		key := strings.ToLower(name) + "\x00" + credit.Role.String()

		position, known := index[key]
		if !known {
			position = len(groups)
			index[key] = position
			groups = append(groups, creditGroup{name: name, role: credit.Role, order: credit.Order})
		}

		if character := strings.TrimSpace(credit.Character); character != "" {
			groups[position].characters = append(groups[position].characters, character)
		}
	}

	return groups
}

func (writer metadataWriter) insertCredits(ctx context.Context, credits []metadata.Credit) error {
	for _, group := range groupCredits(credits) {
		personID, err := writer.personID(ctx, group.name)
		if err != nil {
			return err
		}

		err = writer.queries.InsertItemPerson(ctx, sqlcgen.InsertItemPersonParams{
			ItemID: writer.itemID, PersonID: personID, Role: group.role.String(),
			Character: strings.Join(group.characters, characterSeparator), Position: int64(group.order),
		})
		if err != nil {
			return fmt.Errorf("insert credit: %w", uniqueAsConflict(err))
		}
	}

	return nil
}

func (writer metadataWriter) personID(ctx context.Context, name string) (string, error) {
	id, err := writer.queries.FindPersonID(ctx, name)
	if err == nil {
		return id, nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("find person: %w", err)
	}

	id = uuid.NewV4().String()

	err = writer.queries.InsertPerson(ctx, sqlcgen.InsertPersonParams{ID: id, Name: name, SortName: strings.ToLower(name)})
	if err != nil {
		return "", fmt.Errorf("insert person: %w", uniqueAsConflict(err))
	}

	return id, nil
}

// uniqueAsConflict replaces the driver's UNIQUE violation, whose text names tables and
// columns, with a neutral domain.ErrConflict; other errors pass through.
func uniqueAsConflict(err error) error {
	if isUniqueViolation(err) {
		return fmt.Errorf("a stored row already exists: %w", domain.ErrConflict)
	}

	return err
}

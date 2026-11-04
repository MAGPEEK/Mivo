package sqlite

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// Stored tag kinds (the CHECK of tags.kind).
const (
	tagKindGenre      = "genre"
	tagKindStudio     = "studio"
	tagKindCountry    = "country"
	tagKindCollection = "collection"
)

// readDetailMetadata fills the merged metadata record of an item: its own columns, its tags,
// credits and provider IDs, each in the order of the source.
func readDetailMetadata(ctx context.Context, queries *sqlcgen.Queries, row sqlcgen.Item, detail *applibrary.ItemDetail) error {
	record := metadata.Record{
		Title: row.Title, SortTitle: row.SortTitle, OriginalTitle: row.OriginalTitle, Year: int(row.Year),
		Summary: row.Summary, Tagline: row.Tagline, ContentRating: row.ContentRating,
	}

	if row.ReleaseDate != "" {
		date, err := metadata.ParseDate(row.ReleaseDate)
		if err != nil {
			return undecodable(fmt.Sprintf("item %q: stored release date", row.ID), err)
		}

		record.ReleaseDate = date
	}

	if err := readRecordTags(ctx, queries, row.ID, &record); err != nil {
		return err
	}

	if err := readRecordCredits(ctx, queries, row.ID, &record); err != nil {
		return err
	}

	if err := readRecordProviderIDs(ctx, queries, row.ID, &record); err != nil {
		return err
	}

	detail.Metadata = record

	return nil
}

func readRecordTags(ctx context.Context, queries *sqlcgen.Queries, itemID string, record *metadata.Record) error {
	tags, err := queries.ListItemTags(ctx, itemID)
	if err != nil {
		return fmt.Errorf("read tags: %w", err)
	}

	for _, tag := range tags {
		switch tag.Kind {
		case tagKindGenre:
			record.Genres = append(record.Genres, tag.Name)
		case tagKindStudio:
			record.Studios = append(record.Studios, tag.Name)
		case tagKindCountry:
			record.Countries = append(record.Countries, tag.Name)
		case tagKindCollection:
			record.Collections = append(record.Collections, tag.Name)
		default:
			return undecodable(fmt.Sprintf("item %q: tag %q", itemID, tag.Name), fmt.Errorf("unknown tag kind %q", tag.Kind))
		}
	}

	return nil
}

func readRecordCredits(ctx context.Context, queries *sqlcgen.Queries, itemID string, record *metadata.Record) error {
	credits, err := queries.ListItemCredits(ctx, itemID)
	if err != nil {
		return fmt.Errorf("read credits: %w", err)
	}

	for _, credit := range credits {
		role, err := metadata.ParseRole(credit.Role)
		if err != nil {
			return undecodable(fmt.Sprintf("item %q: credit of %q", itemID, credit.Name), err)
		}

		record.People = append(record.People, metadata.Credit{
			Name: credit.Name, Role: role, Character: credit.Character, Order: int(credit.Position),
		})
	}

	return nil
}

func readRecordProviderIDs(ctx context.Context, queries *sqlcgen.Queries, itemID string, record *metadata.Record) error {
	ids, err := queries.ListItemProviderIDs(ctx, itemID)
	if err != nil {
		return fmt.Errorf("read provider IDs: %w", err)
	}

	for _, stored := range ids {
		provider, err := metadata.ParseProvider(stored.Provider)
		if err != nil {
			return undecodable(fmt.Sprintf("item %q: provider ID", itemID), err)
		}

		record.ProviderIDs = append(record.ProviderIDs, metadata.ProviderID{Provider: provider, Value: stored.ExternalID})
	}

	return nil
}

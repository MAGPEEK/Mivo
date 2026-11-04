// Package metadata holds the pure rules for what an item says about itself (ADR 0027 §6):
// the metadata record with its credits, dates and provider IDs, the lockable fields, the
// metadata sources and the merge of ordered sources that never overwrites a locked field.
//
// The package reads no NFO file and calls no provider; adapters produce one Record per
// source and the scan merges them here. Milestone 6 adds TMDB and the edit dialog on top of
// exactly these rules.
package metadata

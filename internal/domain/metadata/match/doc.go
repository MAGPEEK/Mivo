// Package match holds the pure rules for linking an item to an online provider entry
// (milestone 6, ADR 0031 §9): title normalisation, the confidence score of a candidate against
// an item's parsed name, the auto-match decision (threshold and clear lead), the match states
// and the parsing of pasted provider IDs and URLs.
//
// The package calls no provider and reads no file; adapters produce provider-neutral
// candidates and the use cases apply the decision here.
package match

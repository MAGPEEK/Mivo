// Package watch holds the pure watch state rules of ADR 0032 §8 (task M7-06): how a reported
// playback position changes a profile's state of an item, manual watched marks, which item
// kinds carry watch state, the Continue watching filter and the Up next episode of a show.
//
// Time is always passed in; the package reads no clock. The store (M7-10) and the use cases
// (M7-18, M7-19) only move data, and a store that computes Up next in SQL is tested against
// NextUp as the reference.
package watch

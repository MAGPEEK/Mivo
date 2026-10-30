//go:build race

package sqlite_test

// raceDetectorEnabled is true in builds with -race, which slows the pure Go SQLite driver
// down several times; timing budgets are only logged then.
const raceDetectorEnabled = true

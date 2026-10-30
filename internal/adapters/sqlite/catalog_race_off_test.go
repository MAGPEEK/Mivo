//go:build !race

package sqlite_test

// raceDetectorEnabled is false in builds without -race; timing budgets are enforced.
const raceDetectorEnabled = false

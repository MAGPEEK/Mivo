//go:build e2e

package main

import (
	"strings"
	"time"
)

const (
	// e2eTMDBURLVariable names the origin of the local fake TMDB (API below /3, images below
	// /t/p) of the web E2E suite. It exists only in binaries built with the e2e tag.
	e2eTMDBURLVariable = "MIVO_E2E_TMDB_URL"
	// e2eFakeHost is the only host the fake listens on.
	e2eFakeHost = "127.0.0.1"
	// e2eWorkerPoll lets the enrichment worker notice queued items quickly.
	e2eWorkerPoll = 100 * time.Millisecond
)

// defaultRunHooks points the TMDB adapter at the fake named by the environment, so the E2E
// suite never reaches the internet. Without the variable the hooks stay empty.
func defaultRunHooks(lookupEnv func(key string) (string, bool)) runHooks {
	origin, found := lookupEnv(e2eTMDBURLVariable)
	origin = strings.TrimRight(origin, "/")

	if !found || origin == "" {
		return runHooks{}
	}

	return runHooks{metadata: metadataHooks{
		tmdbBaseURL: origin + "/3", tmdbImageBaseURL: origin + "/t/p",
		extraHosts: []string{e2eFakeHost}, allowInsecureLoopback: true, pollInterval: e2eWorkerPoll,
	}}
}

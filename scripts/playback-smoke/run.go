package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// smokeDependencies are the parts of a run that tests replace.
type smokeDependencies struct {
	httpClient *http.Client
	prober     prober
	poll       pollSettings
	workDir    string
}

// smoke runs the whole check and writes the case lines and the summary to out. It returns
// errCasesFailed when a case did not match and another error when the run could not start.
func smoke(ctx context.Context, opts options, dependencies smokeDependencies, out io.Writer) error {
	manifest, err := readManifest(opts.manifest)
	if err != nil {
		return err
	}

	profile, err := referenceProfile(opts.profile)
	if err != nil {
		return err
	}

	cases, err := smokeCases(manifest, opts.profile)
	if err != nil {
		return err
	}

	client := newAPIClient(opts.base, dependencies.httpClient)

	info, err := client.systemInfo(ctx)
	if err != nil {
		return fmt.Errorf("reach the server at %s: %w", opts.base, err)
	}

	printf(out, "server %s %s at %s, profile %s, %d cases\n", info.ProductName, info.Version, opts.base, opts.profile, len(cases))

	runner := &smokeRunner{
		client: client, prober: dependencies.prober, profile: profile,
		poll: dependencies.poll, caseTimeout: opts.caseTimeout, workDir: dependencies.workDir,
	}

	if err := runner.scanAndIndex(ctx, opts.scanTimeout, out); err != nil {
		return err
	}

	return runner.playAll(ctx, cases, out)
}

// scanAndIndex scans every library and fills the runner's file index.
func (runner *smokeRunner) scanAndIndex(ctx context.Context, timeout time.Duration, out io.Writer) error {
	began := time.Now()

	scanContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	libraries, err := scanLibraries(scanContext, runner.client, runner.poll)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}

	files, err := indexFiles(scanContext, runner.client, libraries)
	if err != nil {
		return fmt.Errorf("index the catalogued files: %w", err)
	}

	printf(out, "scanned %d libraries, %d file names catalogued (%s)\n",
		len(libraries), len(files), time.Since(began).Round(time.Millisecond))

	runner.files = files

	return nil
}

// playAll runs every case, prints its line and the summary. A cancelled run stops after the
// current case and counts the rest as not run.
func (runner *smokeRunner) playAll(ctx context.Context, cases []smokeCase, out io.Writer) error {
	began := time.Now()
	passed, failed := 0, 0

	for _, smoke := range cases {
		if ctx.Err() != nil {
			break
		}

		outcome := runner.runCase(ctx, smoke)
		report(out, smoke, outcome)

		if len(outcome.failures) > 0 {
			failed++
		} else {
			passed++
		}
	}

	notRun := len(cases) - passed - failed

	printf(out, "summary: %d passed, %d failed, %d not run of %d cases (profile %s) in %s\n",
		passed, failed, notRun, len(cases), runner.profile.Name, time.Since(began).Round(time.Second))

	if failed > 0 || notRun > 0 {
		return fmt.Errorf("%w: %d failed, %d not run", errCasesFailed, failed, notRun)
	}

	return nil
}

// printf writes progress output; a failed write to the terminal cannot be reported anywhere.
func printf(out io.Writer, format string, arguments ...any) {
	_, _ = fmt.Fprintf(out, format, arguments...)
}

// Command spkstage stages the Synology Package Toolkit project of one architecture family
// (ADR 0026 section 2, task SPK-14): it renders the templates under packaging/synology/package/
// with the brand values, writes build.env, package.env and the Package Center icons, and copies
// the prebuilt server binary and the family's FFmpeg build into the layout that
// packaging/synology/toolkit/run.sh packs into one SPK.
//
//	go run ./packaging/synology/cmd/spkstage -family x86_64 -version 0.1.0-alpha.2 \
//	    -binary bin/linux-amd64/server -ffmpeg-dir dist/ffmpeg/x86_64 -out dist/stage/x86_64
//
// Every file gets an explicit mode (scripts and executables 0755, everything else 0644,
// directories 0755) and the modification time of -source-date-epoch. Refusals exit with 1 and
// name the input; warnings go to stderr.
package main

import (
	"fmt"
	"os"
)

// exitFailure is the exit status of every refusal.
const exitFailure = 1

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		// A failed write to stderr cannot be reported anywhere else.
		_, _ = fmt.Fprintf(os.Stderr, "spkstage: %v\n", err)
		os.Exit(exitFailure)
	}
}

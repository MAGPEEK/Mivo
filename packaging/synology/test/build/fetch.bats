#!/usr/bin/env bats
# packaging/synology/ffmpeg/fetch.sh (task SPK-16): obtains one family's FFmpeg from the own
# build recipe through `docker build --target <family>` and verifies it against sources.lock.
# Docker is replaced by fake-docker.bash (its --target mode writes a family's recipe output),
# so the cases run in a fraction of a second under the POSIX sh of the test image.
#
# Each test gets its own lock file, recipe directory and destination below $BATS_TEST_TMPDIR.
# shellcheck disable=SC2030,SC2031 # every @test body is its own subshell; the exports are meant for it

FETCH="$(cd "$BATS_TEST_DIRNAME/../../ffmpeg" && pwd)/fetch.sh"
readonly FAKE_VERSION="1.0-static1"

setup() {
	mkdir -p "$BATS_TEST_TMPDIR/bin" "$BATS_TEST_TMPDIR/recipe"
	cp "$BATS_TEST_DIRNAME/fake-docker.bash" "$BATS_TEST_TMPDIR/bin/docker"
	chmod 0755 "$BATS_TEST_TMPDIR/bin/docker"
	export PATH="$BATS_TEST_TMPDIR/bin:$PATH"
	export FAKE_DOCKER_LOG="$BATS_TEST_TMPDIR/docker.log"
	export FFMPEG_RECIPE_DIR="$BATS_TEST_TMPDIR/recipe"
	: >"$FFMPEG_RECIPE_DIR/Dockerfile"
	unset FAKE_FFMPEG_STATUS FAKE_FFMPEG_LICENCE FAKE_FFMPEG_VERSION FAKE_FFMPEG_CHANGED FAKE_FFMPEG_OMIT
	lock="$BATS_TEST_TMPDIR/sources.lock"
	dest="$BATS_TEST_TMPDIR/out"
	write_lock selected
}

fake_hash() { printf 'fake %s %s\n' "$1" "$2" | sha256sum | cut -d' ' -f1; }

# write_lock <status> [version] [ffmpeg-hash-of-armv8] writes the three family entries.
write_lock() {
	local family hash
	{
		echo "# test lock"
		for family in x86_64 armv8 armv7; do
			hash="$(fake_hash ffmpeg "$family")"
			[ "$family" != armv8 ] || hash="${3:-$hash}"
			echo "$family $1 ${2:-$FAKE_VERSION} GPL-3.0-or-later $hash $(fake_hash ffprobe "$family")"
		done
	} >"$lock"
}

run_fetch() { run sh "$FETCH" "$@"; }

# shellcheck disable=SC2154 # status and output are set by bats' run
expect_status() {
	[ "$status" -eq "$1" ] || {
		printf 'expected exit status %s, got %s; output:\n%s\n' "$1" "$status" "$output" >&2
		return 1
	}
}

expect_output() {
	[[ "$output" == *"$1"* ]] || {
		printf 'expected output to contain: %s\noutput:\n%s\n' "$1" "$output" >&2
		return 1
	}
}

expect_no_binaries() {
	[ ! -e "$dest/ffmpeg" ] && [ ! -e "$dest/ffprobe" ] && [ ! -e "$dest/.ffmpeg.partial" ] || {
		printf 'binaries left behind in %s: %s\n' "$dest" "$(ls -A "$dest")" >&2
		return 1
	}
}

@test "installs the verified build with modes and a manifest" {
	run_fetch armv8 "$dest" "$lock"
	expect_status 0
	expect_output "installed FFmpeg $FAKE_VERSION (armv8, selected)"
	grep -q -- "--target armv8" "$FAKE_DOCKER_LOG"
	[ "$(stat -c %a "$dest/ffmpeg")" = 755 ] && [ "$(stat -c %a "$dest/ffprobe")" = 755 ]
	[ "$(stat -c %a "$dest/SOURCE.txt")" = 644 ] && [ "$(stat -c %a "$dest/COPYING.GPLv3")" = 644 ]
	[ "$(cat "$dest/ffmpeg")" = "fake ffmpeg armv8" ]
	[ "$(LC_ALL=C sort "$dest/.fetch-manifest" | tr '\n' ' ')" = "COPYING.GPLv3 SOURCE.txt ffmpeg ffprobe " ]
}

@test "a second fetch removes the files of the first and leaves other files alone" {
	run_fetch armv8 "$dest" "$lock"
	expect_status 0
	echo "mine" >"$dest/keep.txt"
	echo "stale" >"$dest/COPYING.GPLv3"
	export FAKE_FFMPEG_OMIT=COPYING.GPLv3
	run_fetch x86_64 "$dest" "$lock"
	expect_status 0
	[ ! -e "$dest/COPYING.GPLv3" ] && [ -f "$dest/keep.txt" ]
	[ "$(cat "$dest/ffmpeg")" = "fake ffmpeg x86_64" ]
}

@test "a checksum mismatch exits 3 and installs nothing" {
	write_lock selected "$FAKE_VERSION" "$(fake_hash ffmpeg other)"
	run_fetch armv8 "$dest" "$lock"
	expect_status 3
	expect_output "checksum mismatch for ffmpeg"
	expect_no_binaries
}

@test "a notice that contradicts the lock exits 3" {
	write_lock interim
	run_fetch armv8 "$dest" "$lock"
	expect_status 3
	expect_output "SOURCE.txt says Status: 'selected', the lock entry 'interim'"
	expect_no_binaries
	write_lock selected 2.0-static2
	run_fetch armv8 "$dest" "$lock"
	expect_status 3
	expect_output "SOURCE.txt says Version: '$FAKE_VERSION', the lock entry '2.0-static2'"
}

@test "a licence that differs from the lock exits 3, judged on the whole value" {
	local licence
	for licence in GPL-2.0-or-later "GPL-3.0-or-later with nonfree parts"; do
		export FAKE_FFMPEG_LICENCE="$licence"
		run_fetch armv8 "$dest" "$lock"
		expect_status 3
		expect_output "SOURCE.txt says Licence: '$licence', the lock entry 'GPL-3.0-or-later'"
		expect_no_binaries
	done
}

@test "a file missing in the build output exits 4" {
	export FAKE_FFMPEG_OMIT=ffprobe
	run_fetch armv7 "$dest" "$lock"
	expect_status 4
	expect_output "ffprobe missing in the build output"
	expect_no_binaries
}

@test "usage errors exit 2 before docker runs" {
	run_fetch mips "$dest" "$lock"
	expect_status 2
	expect_output "unknown family: mips"
	echo "armv8 selected $FAKE_VERSION GPL-3.0-or-later deadbeef" >"$lock"
	run_fetch armv8 "$dest" "$lock"
	expect_status 2
	expect_output "lock entry of armv8 has 5 fields, expected 6"
	rm "$FFMPEG_RECIPE_DIR/Dockerfile"
	write_lock selected
	run_fetch armv8 "$dest" "$lock"
	expect_status 2
	expect_output "recipe not found"
	[ ! -s "$FAKE_DOCKER_LOG" ]
}

@test "a failed docker build exits 1 and installs nothing" {
	# A docker that fails for every command stands in for a broken recipe build.
	mkdir -p "$BATS_TEST_TMPDIR/nodocker"
	printf '#!/bin/sh\nexit 1\n' >"$BATS_TEST_TMPDIR/nodocker/docker"
	chmod 0755 "$BATS_TEST_TMPDIR/nodocker/docker"
	PATH="$BATS_TEST_TMPDIR/nodocker:$PATH" run sh "$FETCH" armv8 "$dest" "$lock"
	expect_status 1
	expect_output "docker build of the armv8 FFmpeg failed"
	expect_no_binaries
}

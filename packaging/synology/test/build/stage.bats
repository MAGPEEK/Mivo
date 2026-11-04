#!/usr/bin/env bats
# packaging/synology/stage.sh, the staging stage of step 1 (tasks SPK-07, SPK-16), against the
# fake spkstage of fake-spkstage.bash: build-info.txt, the source archive, the one Status: and
# Licence: line per SOURCE.txt and the release refusals (default version, empty revision,
# interim or non-free FFmpeg, spkstage warnings). Runs in the script test image.
# shellcheck disable=SC2030,SC2031 # every @test body is its own subshell; the exports are meant for it

STAGE_SCRIPT="$(cd "$BATS_TEST_DIRNAME/../.." && pwd)/stage.sh"
readonly FAMILIES=(x86_64 armv8 armv7)

setup() {
	prebuilt="$BATS_TEST_TMPDIR/prebuilt"
	out="$BATS_TEST_TMPDIR/out"
	mkdir -p "$BATS_TEST_TMPDIR/bin" "$prebuilt/ffmpeg/source"
	cp "$BATS_TEST_DIRNAME/fake-spkstage.bash" "$BATS_TEST_TMPDIR/bin/spkstage"
	chmod 0755 "$BATS_TEST_TMPDIR/bin/spkstage"
	export PATH="$BATS_TEST_TMPDIR/bin:$PATH"
	unset FAKE_ID FAKE_SPKSTAGE_WARN FAKE_SPKSTAGE_FAIL FAKE_REVISION_SKEW
	export VERSION=1.0.0 REVISION=abc123 PACKAGE_REVISION=0 SOURCE_DATE_EPOCH=0 RELEASE=true \
		NODE_IMAGE=node:test GO_IMAGE=go:test FFMPEG_IMAGE=ffmpeg:test
	local family
	for family in "${FAMILIES[@]}"; do
		mkdir -p "$prebuilt/server/$family" "$prebuilt/ffmpeg/$family"
		echo server >"$prebuilt/server/$family/server"
		echo ffmpeg >"$prebuilt/ffmpeg/$family/ffmpeg"
		echo ffprobe >"$prebuilt/ffmpeg/$family/ffprobe"
		echo licence >"$prebuilt/ffmpeg/$family/COPYING.GPLv3"
		write_notice "$family" "Status:  selected" "Licence: GPL-3.0-or-later"
	done
	echo source >"$prebuilt/ffmpeg/source/ffmpeg-1-source.tar"
	echo "sum  ffmpeg-1-source.tar" >"$prebuilt/ffmpeg/source/ffmpeg-1-source.tar.sha256"
}

# write_notice <family> <line>... writes the family's SOURCE.txt with these lines.
write_notice() {
	local family="$1"
	shift
	printf '%s\n' "FFmpeg build for $family" "$@" >"$prebuilt/ffmpeg/$family/SOURCE.txt"
}

run_stage() { run bash "$STAGE_SCRIPT" "$prebuilt" /src "$out"; }

# shellcheck disable=SC2154 # status and output are set by bats' run
expect_refusal() {
	[ "$status" -eq 1 ] && [[ "$output" == *"$1"* ]] || {
		printf 'expected exit 1 with: %s\nstatus %s, output:\n%s\n' "$1" "$status" "$output" >&2
		return 1
	}
}

expect_success() {
	[ "$status" -eq 0 ] || {
		printf 'expected success, status %s, output:\n%s\n' "$status" "$output" >&2
		return 1
	}
}

@test "a release stages every family, the source archive and build-info.txt" {
	run_stage
	expect_success
	local family
	for family in "${FAMILIES[@]}"; do
		[ -f "$out/$family/build.env" ]
		[ -f "$out/$family/prebuilt/bin/ffmpeg" ]
		grep -qx "ffmpeg_$family=selected GPL-3.0-or-later" "$out/build-info.txt"
	done
	[ -f "$out/ffmpeg-source/ffmpeg-1-source.tar" ]
	[ -f "$out/ffmpeg-source/ffmpeg-1-source.tar.sha256" ]
	grep -qx "revision=abc123" "$out/build-info.txt"
	grep -qx "release=true" "$out/build-info.txt"
	grep -qx "ffmpeg_image=ffmpeg:test" "$out/build-info.txt"
}

@test "a release refuses the default version and an empty revision" {
	VERSION=0.0.0-dev run_stage
	expect_refusal "a release needs VERSION, not the default 0.0.0-dev"
	REVISION="" run_stage
	expect_refusal "a release needs REVISION"
	RELEASE=false VERSION=0.0.0-dev REVISION="" run_stage
	expect_success
}

@test "RELEASE must be true or false" {
	RELEASE=yes run_stage
	expect_refusal "RELEASE must be true or false, not 'yes'"
}

@test "a release refuses an interim or non-free FFmpeg, a test build warns" {
	write_notice armv8 "Status:  interim" "Licence: GPL-3.0-or-later"
	run_stage
	expect_refusal "release refused: the FFmpeg build of armv8 has status 'interim', not 'selected'"
	RELEASE=false run_stage
	expect_success
	[[ "$output" == *"WARNING: armv8 uses an FFmpeg build with status 'interim'"* ]]
	grep -qx "ffmpeg_armv8=interim GPL-3.0-or-later" "$out/build-info.txt"

	write_notice armv8 "Status:  selected" "Licence: GPL-3.0-or-later with nonfree parts  "
	run_stage
	expect_refusal "release refused: the FFmpeg build of armv8 is non-free (GPL-3.0-or-later with nonfree parts)"
}

@test "a SOURCE.txt needs exactly one Status: and one Licence: line in both modes" {
	local mode
	for mode in true false; do
		write_notice x86_64 "Status:  selected" "Status:  interim" "Licence: GPL-3.0-or-later"
		RELEASE=$mode run_stage
		expect_refusal "SOURCE.txt needs exactly one Status: line (has 2)"
		write_notice x86_64 "Status:  selected"
		RELEASE=$mode run_stage
		expect_refusal "SOURCE.txt needs exactly one Licence: line (has 0)"
	done
}

@test "a release refuses an spkstage warning" {
	FAKE_SPKSTAGE_WARN=armv7 run_stage
	expect_refusal "release refused: spkstage warned for armv7"
	FAKE_SPKSTAGE_WARN=armv7 RELEASE=false run_stage
	expect_success
	[[ "$output" == *"spkstage: warning: fake warning for armv7"* ]]
}

@test "a failing spkstage stops the stage with its message" {
	FAKE_SPKSTAGE_FAIL=armv8 RELEASE=false run_stage
	expect_refusal "spkstage failed for armv8"
	[[ "$output" == *"spkstage: fake failure for armv8"* ]]
}

@test "exactly one source archive with its .sha256 is required" {
	echo other >"$prebuilt/ffmpeg/source/ffmpeg-2-source.tar"
	RELEASE=false run_stage
	expect_refusal "holds 2 corresponding source archives in /source, expected 1"
	rm "$prebuilt/ffmpeg/source/"*-source.tar
	RELEASE=false run_stage
	expect_refusal "holds 0 corresponding source archives"
	echo source >"$prebuilt/ffmpeg/source/ffmpeg-1-source.tar"
	rm "$prebuilt/ffmpeg/source/ffmpeg-1-source.tar.sha256"
	RELEASE=false run_stage
	expect_refusal "holds no ffmpeg-1-source.tar.sha256"
}

@test "usage needs three arguments" {
	run bash "$STAGE_SCRIPT" "$prebuilt" /src
	[ "$status" -eq 2 ]
	[[ "$output" == *"usage: stage.sh"* ]]
}

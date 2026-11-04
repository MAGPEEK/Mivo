#!/usr/bin/env bats
# Safety paths of packaging/synology/build.sh and build.ps1 (tasks SPK-07, SPK-16): release
# mode, the interim/non-free FFmpeg refusals, the pins of ffmpeg/sources.lock, the FFmpeg
# source archive, failed toolkit runs and the SPK names. Docker is replaced by fake-docker.bash,
# so each case runs in a fraction of a second. Every case runs against build.sh and, when pwsh
# is installed, against build.ps1 with the same arguments (parity).
#
# Each test copies both scripts into $BATS_TEST_TMPDIR/repo/packaging/synology, next to a
# sources.lock written by the test (its pins are the SHA-256 of the fake binaries fake-docker
# writes), so the real dist/ of the repository is never touched.

SYNOLOGY_DIR="$(cd "$BATS_TEST_DIRNAME/../.." && pwd)"
readonly FAMILIES=(x86_64 armv8 armv7)
readonly FAKE_VERSION="1.0-static1"
readonly SOURCE_ARCHIVE="ffmpeg-$FAKE_VERSION-source.tar"

# Commit recorded by builds in an image without git, where a release needs --revision.
readonly FIXTURE_REVISION="0123456789abcdef0123456789abcdef01234567"

setup() {
	tree="$BATS_TEST_TMPDIR/repo"
	unset CI SPK_BUILD_MODE SPK_PRIVILEGE SOURCE_DATE_EPOCH NODE_IMAGE GO_IMAGE SPK_FFMPEG_IMAGE \
		FAKE_FFMPEG_STATUS FAKE_FFMPEG_LICENCE FAKE_FFMPEG_VERSION FAKE_FFMPEG_CHANGED FAKE_FFMPEG_OMIT \
		FAKE_SOURCE_MISMATCH FAKE_NOTICE_EXTRA FAKE_STAGE_TEST_MODE FAKE_TOOLKIT_FAIL FAKE_TOOLKIT_SILENT \
		FAKE_ID FAKE_SPKSTAGE_WARN FAKE_SPKSTAGE_FAIL FAKE_REVISION_SKEW
	make_tree "$tree"
	mkdir -p "$BATS_TEST_TMPDIR/bin"
	cp "$BATS_TEST_DIRNAME/fake-docker.bash" "$BATS_TEST_TMPDIR/bin/docker"
	cp "$BATS_TEST_DIRNAME/fake-spkstage.bash" "$BATS_TEST_TMPDIR/bin/spkstage"
	chmod 0755 "$BATS_TEST_TMPDIR/bin/docker" "$BATS_TEST_TMPDIR/bin/spkstage"
	export PATH="$BATS_TEST_TMPDIR/bin:$PATH"
	export FAKE_DOCKER_LOG="$BATS_TEST_TMPDIR/docker.log"
	export SPK_TOOLKIT_VOLUME=test-volume
}

have_git() { command -v git >/dev/null 2>&1; }

# make_tree <root> puts copies of the scripts and an all-selected sources.lock below <root>.
# Where git is installed the tree is a clean checkout, as a release needs one; dist/ is ignored
# there as in the repository.
make_tree() {
	script_dir="$1/packaging/synology"
	dist="$script_dir/dist"
	mkdir -p "$script_dir/ffmpeg/build" "$script_dir/toolkit"
	cp "$SYNOLOGY_DIR/build.sh" "$SYNOLOGY_DIR/build.ps1" "$SYNOLOGY_DIR/stage.sh" "$script_dir/"
	if have_git; then
		echo "/dist/" >"$script_dir/.gitignore"
		git -C "$1" init -q
	fi
	write_lock selected
}

# commit_tree commits every change of the test tree, so a release finds it clean.
commit_tree() {
	[ -d "$tree/.git" ] || return 0
	git -C "$tree" add -A
	git -C "$tree" -c user.email=test@example.invalid -c user.name=test commit -qm fixture --allow-empty
}

# install_fake_git <script body> puts a fake git first in PATH; $1 sees git's arguments
# without "-C <dir>".
# shellcheck disable=SC2016 # the script body is written literally
install_fake_git() {
	{
		echo '#!/bin/bash'
		echo '[ "$1" != -C ] || shift 2'
		echo "$1"
	} >"$BATS_TEST_TMPDIR/bin/git"
	chmod 0755 "$BATS_TEST_TMPDIR/bin/git"
}

# fake_hash <tool> <family> is the SHA-256 of the fake binary fake-docker writes.
fake_hash() { printf 'fake %s %s\n' "$1" "$2" | sha256sum | cut -d' ' -f1; }

# write_lock <status> [version] writes one sources.lock entry per family with that status and
# the pins of the fake binaries.
write_lock() {
	local family
	{
		echo "# test lock"
		for family in "${FAMILIES[@]}"; do
			echo "$family $1 ${2:-$FAKE_VERSION} GPL-3.0-or-later $(fake_hash ffmpeg "$family") $(fake_hash ffprobe "$family")"
		done
	} >"$script_dir/ffmpeg/sources.lock"
	commit_tree
}

# run_build <sh|ps1> [build.sh arguments] runs one implementation; for build.ps1 the build.sh
# options are translated to its parameters. Without git a release needs --revision, so the
# fixture commit is added unless the case names a revision itself.
run_build() {
	local implementation="$1" arguments=()
	shift
	if ! have_git && [[ " $* " != *" --revision "* ]]; then
		set -- --revision "$FIXTURE_REVISION" "$@"
	fi
	if [ "$implementation" = sh ]; then
		run bash "$script_dir/build.sh" "$@"
		return
	fi
	while [ "$#" -gt 0 ]; do
		case "$1" in
			--release) arguments+=(-Release) ;;
			--test) arguments+=(-TestBuild) ;;
			--package-revision) arguments+=(-PackageRevision "$2") && shift ;;
			--privilege) arguments+=(-Privilege "$2") && shift ;;
			--revision) arguments+=(-Revision "$2") && shift ;;
			-*) arguments+=("$1") ;;
			*) arguments+=(-Version "$1") ;;
		esac
		shift
	done
	run pwsh -NoProfile -NonInteractive -File "$script_dir/build.ps1" "${arguments[@]}"
}

# shellcheck disable=SC2154 # status and output are set by bats' run
expect_status() {
	[ "$status" -eq "$1" ] || {
		printf 'expected exit status %s, got %s; output:\n%s\n' "$1" "$status" "$output" >&2
		return 1
	}
}

expect_failure() {
	[ "$status" -ne 0 ] || {
		printf 'expected a failure; output:\n%s\n' "$output" >&2
		return 1
	}
}

expect_output() {
	[[ "$output" == *"$1"* ]] || {
		printf 'expected output to contain: %s\noutput:\n%s\n' "$1" "$output" >&2
		return 1
	}
}

expect_log() {
	grep -qF -- "$1" "$FAKE_DOCKER_LOG" 2>/dev/null || {
		printf 'expected a docker call with: %s\ncalls:\n%s\n' "$1" "$(cat "$FAKE_DOCKER_LOG" 2>/dev/null)" >&2
		return 1
	}
}

refute_log() {
	! grep -qF -- "$1" "$FAKE_DOCKER_LOG" 2>/dev/null || {
		printf 'expected no docker call with: %s\ncalls:\n%s\n' "$1" "$(cat "$FAKE_DOCKER_LOG")" >&2
		return 1
	}
}

expect_no_docker() {
	[ ! -s "$FAKE_DOCKER_LOG" ] || {
		printf 'expected no docker call, got:\n%s\n' "$(cat "$FAKE_DOCKER_LOG")" >&2
		return 1
	}
}

expect_spks() {
	local name
	for name in "$@"; do
		[ -f "$dist/$name" ] || {
			printf 'missing %s in dist: %s\n' "$name" "$(ls "$dist")" >&2
			return 1
		}
	done
}

case_ci_defaults_to_release() {
	export CI=true
	write_lock interim
	run_build "$1" 1.0.0
	expect_failure
	expect_output "release refused: ffmpeg/sources.lock has interim FFmpeg entries for x86_64 armv8 armv7"
	expect_no_docker
}

case_ci_values() {
	local value
	write_lock interim
	for value in 1 yes TRUE; do
		export CI="$value"
		run_build "$1" 1.0.0
		expect_failure
		expect_output "release refused"
	done
	for value in false FALSE 0 ""; do
		export CI="$value"
		run_build "$1" 1.0.0
		expect_status 0
		expect_output "WARNING: TEST BUILD WITH INTERIM FFMPEG"
	done
}

case_test_overrides_ci() {
	export CI=true
	write_lock interim
	run_build "$1" --test 1.0.0
	expect_status 0
	expect_output "WARNING: TEST BUILD WITH INTERIM FFMPEG"
	expect_log "RELEASE=false"
	expect_spks demo-1.0.0-x86_64.spk demo-1.0.0-armv8.spk demo-1.0.0-armv7.spk "$SOURCE_ARCHIVE" SHA256SUMS NOT-FOR-RELEASE.txt
}

case_release_refuses_interim_lock() {
	write_lock interim
	run_build "$1" --release 1.0.0
	expect_failure
	expect_output "release refused: ffmpeg/sources.lock has interim FFmpeg entries"
	expect_no_docker
}

case_release_refuses_dev_version() {
	run_build "$1" --release
	expect_failure
	expect_output "a release build needs a version, not the default 0.0.0-dev"
	expect_no_docker
}

# Step 1 (stage.sh) refuses a non-free build of a release; build.sh checks the staged result
# again (FAKE_STAGE_TEST_MODE lets step 1 pass to reach that check).
case_release_refuses_staged_nonfree_ffmpeg() {
	export FAKE_FFMPEG_LICENCE=GPL-3.0-or-later+nonfree
	run_build "$1" --release 1.0.0
	expect_failure
	expect_output "release refused: the FFmpeg build of x86_64 is non-free (GPL-3.0-or-later+nonfree)"
	expect_output "step 1 (docker build) failed"
	expect_log "RELEASE=true"
	refute_log "run "
	export FAKE_STAGE_TEST_MODE=1
	run_build "$1" --release 1.0.0
	expect_failure
	expect_output "release refused: FFmpeg builds not release-eligible"
	expect_output "x86_64 (selected, GPL-3.0-or-later+nonfree)"
	refute_log "run "
	[ ! -e "$dist/SHA256SUMS" ]
}

case_release_refuses_staged_interim_ffmpeg() {
	export FAKE_FFMPEG_STATUS=interim
	run_build "$1" --release 1.0.0
	expect_failure
	expect_output "release refused: the FFmpeg build of x86_64 has status 'interim', not 'selected'"
	refute_log "run "
	export FAKE_STAGE_TEST_MODE=1
	run_build "$1" --release 1.0.0
	expect_failure
	expect_output "x86_64 (interim, GPL-3.0-or-later)"
	refute_log "run "
}

# A warning of spkstage (interim or dynamic FFmpeg) stops a release in step 1; a test build
# shows it and goes on.
case_release_refuses_spkstage_warning() {
	export FAKE_SPKSTAGE_WARN=armv8
	run_build "$1" --release 1.0.0
	expect_failure
	expect_output "spkstage: warning: fake warning for armv8"
	expect_output "release refused: spkstage warned for armv8"
	refute_log "run "
	run_build "$1" --test 1.0.0
	expect_status 0
	expect_output "spkstage: warning: fake warning for armv8"
}

case_release_dies_on_failing_git() {
	install_fake_git 'echo "fatal: detected dubious ownership in repository at /repo" >&2; exit 128'
	run_build "$1" --release 1.0.0
	expect_failure
	expect_output "release refused: git rev-parse --is-inside-work-tree failed"
	expect_output "detected dubious ownership"
	expect_no_docker
	run_build "$1" --release --revision "$FIXTURE_REVISION" 1.0.0
	expect_failure
	expect_output "detected dubious ownership"
	expect_no_docker
	# A test build records no commit and goes on.
	run_build "$1" --test 1.0.0
	expect_status 0
}

case_release_refuses_empty_revision() {
	# shellcheck disable=SC2016 # the fake's script, expanded when it runs
	install_fake_git 'case "$1 $2" in "rev-parse --is-inside-work-tree") echo true ;; esac; exit 0'
	run_build "$1" --release 1.0.0
	expect_failure
	expect_output "release refused: git names no commit for the build (empty revision)"
	expect_no_docker
}

# Without git the commit cannot be read: a release needs --revision and says the tree is not
# checked. Runs where git is missing (the script test image).
case_release_without_git_needs_revision() {
	! have_git || skip "git is installed; the image without git runs this case"
	run_build "$1" --release --revision "$FIXTURE_REVISION" 1.0.0
	expect_status 0
	expect_output "WARNING: git is not installed"
	expect_log "REVISION=$FIXTURE_REVISION"
	grep -qx "revision=$FIXTURE_REVISION" "$dist/stage/build-info.txt"
}

case_release_records_head_commit() {
	have_git || skip "git is not installed"
	run_build "$1" --release 1.0.0
	expect_status 0
	expect_log "REVISION=$(git -C "$tree" rev-parse HEAD)"
}

case_notice_needs_one_line_per_field() {
	local extra
	for extra in "Status:               interim" "Licence:              GPL-3.0-or-later+nonfree" \
		"Version:              2.0-static1" "Source SHA-256:       $(fake_hash x y)"; do
		export FAKE_NOTICE_EXTRA="$extra"
		run_build "$1" --test 1.0.0
		expect_failure
		expect_output "needs exactly one ${extra%%:*}: line"
		refute_log "run "
	done
}

# The staged ffmpeg of one family differs from its pin: a release refuses, a test build packs
# it with a warning and names it in NOT-FOR-RELEASE.txt.
case_release_refuses_unpinned_ffmpeg() {
	export FAKE_FFMPEG_CHANGED=armv8
	run_build "$1" --release 1.0.0
	expect_failure
	expect_output "release refused: FFmpeg builds differ from the pins of ffmpeg/sources.lock: armv8 (ffmpeg)"
	refute_log "run "
	run_build "$1" --test 1.0.0
	expect_status 0
	expect_output "WARNING: FFmpeg builds differ from the pins of ffmpeg/sources.lock"
	grep -q "differ from the pins of ffmpeg/sources.lock: armv8 (ffmpeg)" "$dist/NOT-FOR-RELEASE.txt"
}

# The whole Licence: value counts: another licence than the pinned one is refused, and a
# "nonfree" anywhere on the line makes the build ineligible, not only in the first word.
case_release_refuses_licence_mismatch() {
	export FAKE_FFMPEG_LICENCE=GPL-2.0-or-later
	run_build "$1" --release 1.0.0
	expect_failure
	expect_output "x86_64 (licence GPL-2.0-or-later (pinned GPL-3.0-or-later))"
	refute_log "run "
	export FAKE_FFMPEG_LICENCE="GPL-3.0-or-later with nonfree parts" FAKE_STAGE_TEST_MODE=1
	run_build "$1" --release 1.0.0
	expect_failure
	expect_output "release refused: FFmpeg builds not release-eligible"
	expect_output "x86_64 (selected, GPL-3.0-or-later with nonfree parts)"
	refute_log "run "
}

case_release_refuses_pin_version_mismatch() {
	write_lock selected 2.0-static2
	run_build "$1" --release 1.0.0
	expect_failure
	expect_output "x86_64 (version $FAKE_VERSION (pinned 2.0-static2))"
	refute_log "run "
}

# The source archive step 1 staged must be the one every SOURCE.txt names, in both modes.
case_source_archive_mismatch_fails() {
	export FAKE_SOURCE_MISMATCH=1
	run_build "$1" --test 1.0.0
	expect_failure
	expect_output "is not the one SOURCE.txt of x86_64 names"
	refute_log "run "
	[ ! -e "$dist/$SOURCE_ARCHIVE" ]
}

case_malformed_lock_refused() {
	echo "x86_64 selected $FAKE_VERSION GPL-3.0-or-later deadbeef" >"$script_dir/ffmpeg/sources.lock"
	run_build "$1" --test 1.0.0
	expect_failure
	expect_output "ffmpeg/sources.lock: the entry of x86_64 has 5 fields, expected 6"
	expect_no_docker
	write_lock selected
	echo "armv8 selected $FAKE_VERSION GPL-3.0-or-later $(fake_hash ffmpeg armv8) $(fake_hash ffprobe armv8)" >>"$script_dir/ffmpeg/sources.lock"
	run_build "$1" --test 1.0.0
	expect_failure
	expect_output "ffmpeg/sources.lock has two entries for armv8"
	expect_no_docker
}

# Step 0 builds the FFmpeg recipe image before step 1, which receives its name.
case_ffmpeg_image_built_before_stage() {
	export SPK_FFMPEG_IMAGE=test-ffmpeg:dev
	run_build "$1" --test 1.0.0
	expect_status 0
	expect_log "build -t test-ffmpeg:dev"
	expect_log "FFMPEG_IMAGE=test-ffmpeg:dev"
	[ "$(grep -n -- '-t test-ffmpeg:dev' "$FAKE_DOCKER_LOG" | cut -d: -f1)" -lt \
		"$(grep -n -- 'FFMPEG_IMAGE=test-ffmpeg:dev' "$FAKE_DOCKER_LOG" | cut -d: -f1)" ]
	expect_output "step 0: FFmpeg"
}

case_failed_toolkit_run_writes_no_sums() {
	export FAKE_TOOLKIT_FAIL=armv8
	run_build "$1" --test 1.0.0
	expect_failure
	expect_output "[spk-toolkit] failed: fake toolkit failure for armv8"
	expect_output "toolkit run for armv8 failed"
	expect_spks demo-1.0.0-x86_64.spk
	[ ! -e "$dist/SHA256SUMS" ]
	if grep -q ' armv7$' "$FAKE_DOCKER_LOG"; then
		echo "armv7 was packed after the failed armv8 run" >&2
		return 1
	fi
}

case_release_build_names_and_sums() {
	run_build "$1" --release 1.0.0
	expect_status 0
	expect_log "RELEASE=true"
	expect_spks demo-1.0.0-x86_64.spk demo-1.0.0-armv8.spk demo-1.0.0-armv7.spk "$SOURCE_ARCHIVE" SHA256SUMS
	[ ! -e "$dist/NOT-FOR-RELEASE.txt" ]
	[ ! -e "$dist/stage/ffmpeg-source/$SOURCE_ARCHIVE" ]
	(cd "$dist" && sha256sum -c SHA256SUMS)
	[ "$(cut -d' ' -f3 "$dist/SHA256SUMS" | tr '\n' ' ')" = "demo-1.0.0-armv7.spk demo-1.0.0-armv8.spk demo-1.0.0-x86_64.spk $SOURCE_ARCHIVE " ]
	expect_output "FFmpeg corresponding source: $SOURCE_ARCHIVE"
}

case_package_revision_in_name() {
	run_build "$1" --test --package-revision 1 0.1.0-alpha.2
	expect_status 0
	expect_log "PACKAGE_REVISION=1"
	expect_spks demo-0.1.0-alpha.2-r1-x86_64.spk demo-0.1.0-alpha.2-r1-armv8.spk demo-0.1.0-alpha.2-r1-armv7.spk
}

case_privilege_options() {
	run_build "$1" --test 1.0.0
	expect_status 0
	expect_log "run --rm --cap-add SYS_ADMIN -v test-volume:/toolkit"
	refute_log "--privileged"
	: >"$FAKE_DOCKER_LOG"
	run_build "$1" --test --privilege privileged 1.0.0
	expect_status 0
	expect_log "run --rm --privileged -v"
	: >"$FAKE_DOCKER_LOG"
	export SPK_PRIVILEGE=sys-admin-apparmor
	run_build "$1" --test 1.0.0
	expect_status 0
	expect_log "run --rm --cap-add SYS_ADMIN --security-opt apparmor=unconfined -v"
	run_build "$1" --test --privilege root 1.0.0
	expect_failure
	expect_output "must be sys-admin, sys-admin-apparmor or privileged"
}

case_version_checked_early() {
	local version
	for version in v1.0.0 1.0 1.0.0-alpha.01 1.0.0-gamma.1 01.0.0 "1.0.0+build"; do
		run_build "$1" --test "$version"
		expect_failure
		expect_output "is not <major>.<minor>.<patch>"
	done
	expect_no_docker
}

# expect_value_refused <sh|ps1> <build.sh option> <build.ps1 parameter>: the last run refused
# the value of that option, in each implementation's words.
expect_value_refused() {
	expect_failure
	if [ "$1" = sh ]; then
		expect_output "$2 needs a value"
	else
		# PowerShell's binder refuses it: "Cannot validate argument" or "Missing an argument".
		expect_output "parameter '$3'"
	fi
}

case_empty_option_values_refused() {
	run_build "$1" --test --revision "" 1.0.0
	expect_value_refused "$1" --revision Revision
	run_build "$1" --test --privilege "" 1.0.0
	expect_value_refused "$1" --privilege Privilege
	expect_no_docker
}

# A value that starts with "-" is an option that lost its value.
case_option_values_starting_with_dash_refused() {
	run_build "$1" --test --revision --release 1.0.0
	expect_value_refused "$1" --revision Revision
	run_build "$1" --test --privilege -privileged 1.0.0
	expect_value_refused "$1" --privilege Privilege
	expect_no_docker
}

# The package name comes from the staged build.env, whatever it is; an invalid one stops the build.
case_package_name_from_build_env() {
	export FAKE_ID=other
	run_build "$1" --test 1.0.0
	expect_status 0
	expect_spks other-1.0.0-x86_64.spk other-1.0.0-armv8.spk other-1.0.0-armv7.spk
	export FAKE_ID="-bad"
	: >"$FAKE_DOCKER_LOG"
	run_build "$1" --test 1.0.0
	expect_failure
	expect_output "staged build.env of x86_64 has no valid ID ('-bad')"
	refute_log "run "
}

case_staged_package_revision_must_match() {
	export FAKE_REVISION_SKEW=1
	run_build "$1" --test --package-revision 2 1.0.0
	expect_failure
	expect_output "staged DSM_VERSION 1.0.0-9003 of x86_64 encodes package revision 3, not 2"
	refute_log "run "
}

case_toolkit_that_writes_nothing_fails() {
	export FAKE_TOOLKIT_SILENT=armv8
	run_build "$1" --test 1.0.0
	expect_failure
	expect_output "the toolkit did not write demo-armv8-1.0.0-9000.spk"
	[ ! -e "$dist/SHA256SUMS" ]
}

case_stage_path_with_comma_and_blank() {
	tree="$BATS_TEST_TMPDIR/a, b/repo"
	make_tree "$tree"
	run_build "$1" --test 1.0.0
	expect_status 0
	expect_spks demo-1.0.0-x86_64.spk SHA256SUMS
}

case_release_refuses_dirty_tree() {
	have_git || skip "git not installed"
	# make_tree committed the tree; dist/ is ignored and never makes it dirty.
	run_build "$1" --release 1.0.0
	expect_status 0
	touch "$tree/untracked"
	run_build "$1" --release 1.0.0
	expect_failure
	expect_output "release refused: the working tree has uncommitted changes"
	expect_output "untracked"
}

CASES=(
	ci_defaults_to_release ci_values test_overrides_ci release_refuses_interim_lock
	release_refuses_dev_version release_refuses_staged_nonfree_ffmpeg
	release_refuses_staged_interim_ffmpeg release_refuses_spkstage_warning release_dies_on_failing_git
	release_refuses_empty_revision release_without_git_needs_revision release_records_head_commit
	notice_needs_one_line_per_field option_values_starting_with_dash_refused package_name_from_build_env
	staged_package_revision_must_match toolkit_that_writes_nothing_fails
	release_refuses_unpinned_ffmpeg release_refuses_licence_mismatch release_refuses_pin_version_mismatch
	source_archive_mismatch_fails malformed_lock_refused ffmpeg_image_built_before_stage
	failed_toolkit_run_writes_no_sums release_build_names_and_sums package_revision_in_name
	privilege_options version_checked_early empty_option_values_refused
	stage_path_with_comma_and_blank release_refuses_dirty_tree
)

IMPLEMENTATIONS=(sh)
if command -v pwsh >/dev/null 2>&1; then
	IMPLEMENTATIONS+=(ps1)
else
	bats_test_function --description "build.ps1 parity cases" -- \
		skip "pwsh is not installed; run the parity image of test/build/Dockerfile (test/README.md)"
fi
for implementation in "${IMPLEMENTATIONS[@]}"; do
	for case_name in "${CASES[@]}"; do
		bats_test_function --description "${case_name//_/ } [build.$implementation]" -- \
			"case_$case_name" "$implementation"
	done
done

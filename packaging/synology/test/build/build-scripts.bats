#!/usr/bin/env bats
# Safety paths of packaging/synology/build.sh and build.ps1 (task SPK-07): release mode, the
# interim/non-free FFmpeg refusals, failed toolkit runs and the SPK names. Docker is replaced by
# fake-docker.bash, so each case runs in a fraction of a second. Every case runs against
# build.sh and, when pwsh is installed, against build.ps1 with the same arguments (parity).
#
# Each test copies both scripts into $BATS_TEST_TMPDIR/repo/packaging/synology, next to a
# sources.lock written by the test, so the real dist/ of the repository is never touched.

SYNOLOGY_DIR="$(cd "$BATS_TEST_DIRNAME/../.." && pwd)"
readonly FAMILIES=(x86_64 armv8 armv7)

setup() {
	tree="$BATS_TEST_TMPDIR/repo"
	make_tree "$tree"
	mkdir -p "$BATS_TEST_TMPDIR/bin"
	cp "$BATS_TEST_DIRNAME/fake-docker.bash" "$BATS_TEST_TMPDIR/bin/docker"
	chmod 0755 "$BATS_TEST_TMPDIR/bin/docker"
	export PATH="$BATS_TEST_TMPDIR/bin:$PATH"
	export FAKE_DOCKER_LOG="$BATS_TEST_TMPDIR/docker.log"
	unset CI SPK_BUILD_MODE SPK_PRIVILEGE SOURCE_DATE_EPOCH NODE_IMAGE GO_IMAGE FFMPEG_FETCH_IMAGE \
		FAKE_FFMPEG_STATUS FAKE_FFMPEG_LICENCE FAKE_NOTICE_EXTRA FAKE_TOOLKIT_FAIL
	export SPK_TOOLKIT_VOLUME=test-volume
}

# make_tree <root> puts copies of the scripts and an all-selected sources.lock below <root>.
make_tree() {
	script_dir="$1/packaging/synology"
	dist="$script_dir/dist"
	mkdir -p "$script_dir/ffmpeg" "$script_dir/toolkit"
	cp "$SYNOLOGY_DIR/build.sh" "$SYNOLOGY_DIR/build.ps1" "$script_dir/"
	write_lock selected
}

# write_lock <status> writes one sources.lock entry per family with that status.
write_lock() {
	local family
	{
		echo "# test lock"
		for family in "${FAMILIES[@]}"; do
			echo "$family $1 1.0 GPL-3.0-or-later https://example.invalid/a.tar.xz $(printf '0%.0s' {1..64}) . https://example.invalid/s.tar.xz $(printf '0%.0s' {1..64}) archive:COPYING"
		done
	} >"$script_dir/ffmpeg/sources.lock"
}

# run_build <sh|ps1> [build.sh arguments] runs one implementation; for build.ps1 the build.sh
# options are translated to its parameters.
run_build() {
	local implementation="$1" arguments=()
	shift
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
	expect_spks demo-1.0.0-x86_64.spk demo-1.0.0-armv8.spk demo-1.0.0-armv7.spk SHA256SUMS NOT-FOR-RELEASE.txt
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

case_release_refuses_staged_nonfree_ffmpeg() {
	export FAKE_FFMPEG_LICENCE=GPL-3.0-or-later+nonfree
	run_build "$1" --release 1.0.0
	expect_failure
	expect_output "release refused: FFmpeg builds not release-eligible"
	expect_output "x86_64 (selected, GPL-3.0-or-later+nonfree)"
	expect_log "RELEASE=true"
	refute_log "run "
	[ ! -e "$dist/SHA256SUMS" ]
}

case_release_refuses_staged_interim_ffmpeg() {
	export FAKE_FFMPEG_STATUS=interim
	run_build "$1" --release 1.0.0
	expect_failure
	expect_output "x86_64 (interim, GPL-3.0-or-later)"
	refute_log "run "
}

case_notice_needs_one_status_and_licence() {
	local extra
	for extra in "Status:               interim" "Licence:              GPL-3.0-or-later+nonfree"; do
		export FAKE_NOTICE_EXTRA="$extra"
		run_build "$1" --test 1.0.0
		expect_failure
		expect_output "needs exactly one Status: and one Licence: line"
		refute_log "run "
	done
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
	expect_spks demo-1.0.0-x86_64.spk demo-1.0.0-armv8.spk demo-1.0.0-armv7.spk SHA256SUMS
	[ ! -e "$dist/NOT-FOR-RELEASE.txt" ]
	(cd "$dist" && sha256sum -c SHA256SUMS)
	[ "$(cut -d' ' -f3 "$dist/SHA256SUMS" | tr '\n' ' ')" = "demo-1.0.0-armv7.spk demo-1.0.0-armv8.spk demo-1.0.0-x86_64.spk " ]
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

case_empty_option_values_refused() {
	run_build "$1" --test --revision "" 1.0.0
	expect_failure
	run_build "$1" --test --privilege "" 1.0.0
	expect_failure
	expect_no_docker
}

case_stage_path_with_comma_and_blank() {
	tree="$BATS_TEST_TMPDIR/a, b/repo"
	make_tree "$tree"
	run_build "$1" --test 1.0.0
	expect_status 0
	expect_spks demo-1.0.0-x86_64.spk SHA256SUMS
}

case_release_refuses_dirty_tree() {
	command -v git >/dev/null 2>&1 || skip "git not installed"
	# As in the repository, dist/ is ignored and never makes the tree dirty.
	echo "/dist/" >"$script_dir/.gitignore"
	git -C "$tree" init -q
	git -C "$tree" add -A
	git -C "$tree" -c user.email=test@example.invalid -c user.name=test commit -qm fixture
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
	release_refuses_staged_interim_ffmpeg notice_needs_one_status_and_licence
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

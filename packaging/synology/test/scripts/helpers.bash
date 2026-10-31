# shellcheck shell=bash
# shellcheck disable=SC2034 # the constants and paths are used by the .bats files loading this
# Shared helpers of the package script tests (../README.md): a DSM-like package tree per test,
# the fake server and functions that run a package script under the shell of the current case.

bats_load_library bats-support
bats_load_library bats-assert

REPO_ROOT=$(cd "$BATS_TEST_DIRNAME/../../../.." && pwd)
PACKAGE_SCRIPTS=$REPO_ROOT/packaging/synology/package/scripts
TEST_DIR=$REPO_ROOT/packaging/synology/test

# Shells every case runs under. DSM's /bin/sh is unconfirmed (DSM-NOTES section 5): BusyBox
# ash stands for a minimal POSIX shell, "bash --posix" for /bin/sh being a link to bash, dash
# for a strict POSIX shell without bash or BusyBox extensions. SCRIPT_TEST_SHELLS replaces the
# list for a one-off run (comma-separated, e.g. "dash").
if [[ -n ${SCRIPT_TEST_SHELLS:-} ]]; then
	IFS=, read -ra SCRIPT_SHELLS <<<"$SCRIPT_TEST_SHELLS"
else
	SCRIPT_SHELLS=("busybox sh" "bash --posix" "bash" "dash")
fi

# Name of the package in the fixture package.env (fixtures/package.env).
readonly FIXTURE_ID=demo
readonly FIXTURE_DISPLAY_NAME="Demo Media Server"

# Seconds after which bats fails a test, so that a hang ends the run instead of blocking it.
# The slowest case needs about 10 s.
BATS_TEST_TIMEOUT=${BATS_TEST_TIMEOUT:-60}

# Seconds a test waits for something the fake does in the background.
readonly EVENTUALLY_SECONDS=5

# Lines a script may print per action on stdout and stderr together.
readonly SHORT_OUTPUT_MAX_LINES=3

# register_cases registers each case function case_<name> once per shell of SCRIPT_SHELLS. A
# shell that is not installed gets one failing test instead of its cases, so a run in an image
# without it cannot pass silently.
register_cases() {
	local script_shell case_name
	for script_shell in "${SCRIPT_SHELLS[@]}"; do
		if ! command -v "${script_shell%% *}" >/dev/null; then
			bats_test_function --description "shell ${script_shell%% *} is installed" -- \
				fail "${script_shell%% *} is missing; run the tests in the image of ../Dockerfile"
			continue
		fi
		for case_name in "$@"; do
			bats_test_function --description "${case_name//_/ } [$script_shell]" -- \
				run_case "$script_shell" "case_$case_name"
		done
	done
}

run_case() {
	CASE_SHELL=$1
	"$2"
}

# setup_package_tree builds what DSM gives a package (DSM-NOTES section 8): target is a link
# into @appstore as on DSM, var, tmp and home are directories, scripts holds every package
# script with the fixture package.env. The package binary is the fake server
# (fake-server.bash). $BATS_TEST_TMPDIR stands for / of the NAS: it holds volume1, and the
# scripts look for shared folders below it (PKG_TEST_VOLUMES_ROOT).
setup_package_tree() {
	local volume=$BATS_TEST_TMPDIR/volume1
	PKG_ROOT=$BATS_TEST_TMPDIR/packages/$FIXTURE_ID
	mkdir -p "$volume/@appstore/$FIXTURE_ID/bin" "$PKG_ROOT"/{scripts,var,tmp,home}
	ln -s "$volume/@appstore/$FIXTURE_ID" "$PKG_ROOT/target"
	cp "$PACKAGE_SCRIPTS"/* "$PKG_ROOT/scripts/"
	cp "$TEST_DIR/fixtures/package.env" "$PKG_ROOT/scripts/"
	cp "$(command -v bash)" "$PKG_ROOT/target/bin/$FIXTURE_ID"
	printf '#!/bin/sh\nexit 0\n' >"$PKG_ROOT/target/bin/ffmpeg"
	cp "$PKG_ROOT/target/bin/ffmpeg" "$PKG_ROOT/target/bin/ffprobe"
	chmod 0755 "$PKG_ROOT/target/bin/"*

	PKG_LOG=$PKG_ROOT/var/$FIXTURE_ID.log
	PKG_PID=$PKG_ROOT/var/$FIXTURE_ID.pid
	TEMP_LOGFILE=$BATS_TEST_TMPDIR/synopkg-temp.log
	FAKE_STATE_DIR=$BATS_TEST_TMPDIR/fake
	FOREIGN_PIDS=()
	mkdir -p "$FAKE_STATE_DIR"

	export SYNOPKG_PKGNAME=$FIXTURE_ID SYNOPKG_PKG_STATUS=START
	export SYNOPKG_PKGDEST=$PKG_ROOT/target SYNOPKG_PKGVAR=$PKG_ROOT/var
	export SYNOPKG_PKGTMP=$PKG_ROOT/tmp SYNOPKG_PKGHOME=$PKG_ROOT/home
	export SYNOPKG_TEMP_LOGFILE=$TEMP_LOGFILE
	export PKG_TEST_VOLUMES_ROOT=$BATS_TEST_TMPDIR
	export BASH_ENV=$TEST_DIR/fake-server.bash FAKE_BINARY_NAME=$FIXTURE_ID FAKE_STATE_DIR
	# Wizard keys and the DSM language are set by the cases that need them.
	unset wizard_media_share wizard_delete_data SYNOPKG_DSM_LANGUAGE
}

# stop_leftover_processes kills every fake server and foreign process a test started.
stop_leftover_processes() {
	local pid
	if [[ -f $FAKE_STATE_DIR/server.starts ]]; then
		while read -r pid; do
			kill -KILL "$pid" 2>/dev/null || :
		done <"$FAKE_STATE_DIR/server.starts"
	fi
	# A child that never became the server is not in server.starts; a faulty script may still
	# have recorded it.
	if [[ -f $PKG_PID ]] && read -r pid <"$PKG_PID" && [[ $pid =~ ^[0-9]+$ ]] &&
		[[ " ${FOREIGN_PIDS[*]} " != *" $pid "* ]]; then
		kill -KILL "$pid" 2>/dev/null || :
	fi
	# Foreign processes end on SIGTERM; SIGKILL would make bash report them as killed.
	for pid in "${FOREIGN_PIDS[@]}"; do
		kill -TERM "$pid" 2>/dev/null || :
	done
}

# run_script runs the package script at path $1 with the remaining arguments under the shell of
# the current case; stdout lands in $output/$lines, stderr in $stderr/$stderr_lines.
run_script() {
	local -a shell_command
	read -ra shell_command <<<"$CASE_SHELL"
	run --separate-stderr "${shell_command[@]}" "$@"
}

# run_sss runs start-stop-status of the package tree with the given arguments.
run_sss() {
	run_script "$PKG_ROOT/scripts/start-stop-status" "$@"
}

# run_common runs the common.sh function $1 with the remaining arguments under the shell of the
# current case, after loading common.sh and the fixture package.env as the scripts do.
run_common() {
	local -a shell_command
	read -ra shell_command <<<"$CASE_SHELL"
	# shellcheck disable=SC2016 # expanded by the shell under test
	run --separate-stderr "${shell_command[@]}" -c \
		'PKG_SCRIPT_DIR=$1; shift; . "$PKG_SCRIPT_DIR/common.sh" && pkg_load_package_env && "$@"' \
		_ "$PKG_ROOT/scripts" "$@"
}

# fake_failing_command puts a command $1 first in PATH that fails when one of its arguments
# matches the shell pattern $2 and runs the real command otherwise. As root in the container,
# rm and mv cannot be made to fail through file permissions.
fake_failing_command() {
	local name=$1 pattern=$2 real fake_dir=$BATS_TEST_TMPDIR/fake-bin
	real=$(command -v "$name")
	mkdir -p "$fake_dir"
	cat >"$fake_dir/$name" <<EOF
#!/bin/sh
for argument in "\$@"; do
	case "\$argument" in
	$pattern) echo "$name: refused \$argument (test)" >&2; exit 1 ;;
	esac
done
exec "$real" "\$@"
EOF
	chmod 0755 "$fake_dir/$name"
	export PATH=$fake_dir:$PATH
}

fake_mode() {
	echo "$1" >"$FAKE_STATE_DIR/mode"
}

server_starts() {
	if [[ -f $FAKE_STATE_DIR/server.starts ]]; then
		wc -l <"$FAKE_STATE_DIR/server.starts" | tr -d ' '
	else
		echo 0
	fi
}

# process_running succeeds while process $1 exists and is not a zombie.
process_running() {
	local stat
	read -r stat 2>/dev/null <"/proc/$1/stat" || return 1
	stat=${stat##*) }
	[[ ${stat%% *} != [ZX] ]]
}

# start_foreign_process starts a process that is not the package binary and sets FOREIGN_PID.
# It is a bash, like the fake server's binary, but another file: only the exact binary counts.
start_foreign_process() {
	bash -c 'trap "exit 0" TERM; while :; do sleep 0.2; done' </dev/null >/dev/null 2>&1 3>&- &
	FOREIGN_PID=$!
	track_foreign_process "$FOREIGN_PID"
}

# track_foreign_process makes stop_leftover_processes end process $1 after the test.
track_foreign_process() {
	FOREIGN_PIDS+=("$1")
}

# dead_pid prints the PID of a process that has already exited and been reaped.
dead_pid() {
	local pid
	true 3>&- &
	pid=$!
	wait "$pid"
	echo "$pid"
}

# eventually retries the command for up to EVENTUALLY_SECONDS.
eventually() {
	eventually_within "$EVENTUALLY_SECONDS" "$@"
}

# eventually_within retries the command (from $2 on) for up to $1 seconds.
eventually_within() {
	local seconds=$1 attempt
	shift
	for ((attempt = 0; attempt < seconds * 10; attempt++)); do
		if "$@"; then
			return 0
		fi
		sleep 0.1
	done
	"$@"
}

# monotonic_centiseconds prints the time since boot from /proc/uptime in hundredths of a second.
# Timing assertions use it instead of SECONDS or `date +%s`: under Docker Desktop (WSL2) the
# container's wall clock can lag behind the time that really passed.
monotonic_centiseconds() {
	local uptime rest
	read -r uptime rest </proc/uptime
	[[ $uptime =~ ^([0-9]+)\.([0-9]{2})$ ]] || fail "unexpected /proc/uptime: $uptime $rest"
	echo $((10#${BASH_REMATCH[1]} * 100 + 10#${BASH_REMATCH[2]}))
}

# elapsed_centiseconds prints the hundredths of a second since monotonic_centiseconds printed $1.
elapsed_centiseconds() {
	local now
	now=$(monotonic_centiseconds)
	echo $((now - $1))
}

format_centiseconds() {
	printf '%d.%02d' $(($1 / 100)) $(($1 % 100))
}

# assert_elapsed_at_least fails with message $3 unless $2 whole seconds have passed since the
# monotonic time $1.
assert_elapsed_at_least() {
	local elapsed
	elapsed=$(elapsed_centiseconds "$1")
	if ((elapsed < $2 * 100)); then
		fail "$3 (after $(format_centiseconds "$elapsed") s, expected at least $2 s)"
	fi
}

# assert_elapsed_below fails with message $3 unless less than $2 whole seconds have passed since
# the monotonic time $1.
assert_elapsed_below() {
	local elapsed
	elapsed=$(elapsed_centiseconds "$1")
	if ((elapsed >= $2 * 100)); then
		fail "$3 (after $(format_centiseconds "$elapsed") s, expected less than $2 s)"
	fi
}

assert_file_line() {
	local file=$1 expected=$2
	if ! grep -qxF -- "$expected" "$file"; then
		fail "$file has no line '$expected'; content:"$'\n'"$(cat "$file")"
	fi
}

refute_file_line() {
	local file=$1 unexpected=$2
	if grep -qxF -- "$unexpected" "$file"; then
		fail "$file has the line '$unexpected'"
	fi
}

# refute_error_words fails when the last run printed words that the toolkit's log check and a
# reader of DSM's package log take for a failure.
# shellcheck disable=SC2154 # output and stderr are set by bats' run
refute_error_words() {
	if grep -qE 'Error|error:|fatal error' <<<"$output"$'\n'"$stderr"; then
		fail "the script printed an error word:"$'\n'"$output"$'\n'"$stderr"
	fi
}

assert_stderr_contains() {
	if [[ $stderr != *"$1"* ]]; then
		fail "stderr lacks '$1'; stderr:"$'\n'"$stderr"
	fi
}

# assert_short_output fails when the script printed more than a few lines on stdout and stderr
# together: DSM appends both to /var/log/packages/<id>.log.
# shellcheck disable=SC2154 # lines, stderr_lines, output and stderr are set by bats' run
assert_short_output() {
	local total=$((${#lines[@]} + ${#stderr_lines[@]}))
	if ((total > SHORT_OUTPUT_MAX_LINES)); then
		fail "script printed $total lines, at most $SHORT_OUTPUT_MAX_LINES expected:"$'\n'"$output"$'\n'"$stderr"
	fi
}

assert_mode() {
	local path=$1 expected=$2 actual
	actual=$(stat -c %a "$path")
	if [[ $actual != "$expected" ]]; then
		fail "$path has mode $actual, expected $expected"
	fi
}

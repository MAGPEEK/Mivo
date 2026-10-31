#!/usr/bin/env bats
# Tests of the service control script start-stop-status and of common.sh (task SPK-05). Each
# case runs once per shell of SCRIPT_SHELLS (helpers.bash) against a fake server; how to run
# them: ../README.md.

bats_require_minimum_version 1.11.0

load helpers

setup() {
	setup_package_tree
}

teardown() {
	stop_leftover_processes
}

pid_from_file() {
	cat "$PKG_PID"
}

# --- start ---------------------------------------------------------------------------------

case_start_healthy_server() {
	run_sss start
	assert_success
	assert_output --partial "$FIXTURE_DISPLAY_NAME is running"
	assert_short_output
	local pid
	pid=$(pid_from_file)
	assert_equal "$(cat "$FAKE_STATE_DIR/server.starts")" "$pid"
	process_running "$pid"

	run_sss status
	assert_success
	assert_output ''
}

case_start_exports_package_environment() {
	run_sss start
	assert_success
	local env=$FAKE_STATE_DIR/server.env
	assert_file_line "$env" "DEMO_CONFIG_DIR=$SYNOPKG_PKGVAR"
	assert_file_line "$env" "DEMO_PATHS_DATA=$SYNOPKG_PKGVAR/data"
	assert_file_line "$env" "DEMO_PATHS_CACHE=$SYNOPKG_PKGVAR/cache"
	assert_file_line "$env" "DEMO_PATHS_TRANSCODE=$SYNOPKG_PKGTMP/transcode"
	assert_file_line "$env" "DEMO_TOOLS_FFMPEG=$SYNOPKG_PKGDEST/bin/ffmpeg"
	assert_file_line "$env" "DEMO_TOOLS_FFPROBE=$SYNOPKG_PKGDEST/bin/ffprobe"
	assert_file_line "$env" "HOME=$SYNOPKG_PKGHOME"
	# Port, LAN guard and libraries stay in config.yaml (ADR 0026 section 6).
	run grep -E '^DEMO_(SERVER_|LIBRARIES|LOG_)' "$env"
	assert_failure
	# No other prefix than the one of package.env.
	local variable
	while read -r variable; do
		[[ $variable == DEMO_* ]] || fail "server got $variable, not built from ENV_PREFIX"
	done < <(grep -E '^[A-Z0-9_]+_(CONFIG_DIR|PATHS_[A-Z]+|TOOLS_[A-Z]+)=' "$env")
	# The server gets no arguments; the healthcheck the same environment and only its command.
	assert_equal "$(cat "$FAKE_STATE_DIR/server.args")" ""
	assert_file_line "$FAKE_STATE_DIR/healthcheck.env" "DEMO_CONFIG_DIR=$SYNOPKG_PKGVAR"
	assert_equal "$(cat "$FAKE_STATE_DIR/healthcheck.args")" ""
}

case_start_sends_server_output_to_its_log_only() {
	run_sss start
	assert_success
	assert_file_line "$PKG_LOG" "fake server: starting in mode healthy"
	assert_file_line "$PKG_LOG" "fake server: diagnostics on stderr"
	refute_output --partial "fake server"
	[[ $stderr != *"fake server"* ]] || fail "server output reached the script's stderr"
	assert_mode "$PKG_LOG" 640
}

case_start_creates_server_directories() {
	run_sss start
	assert_success
	assert_mode "$SYNOPKG_PKGVAR/data" 750
	assert_mode "$SYNOPKG_PKGVAR/cache" 750
	assert_mode "$SYNOPKG_PKGTMP/transcode" 750
}

case_start_removes_stale_transcode_output() {
	local transcode=$SYNOPKG_PKGTMP/transcode
	mkdir -p "$transcode/session-1/variant" "$SYNOPKG_PKGVAR/data"
	touch "$transcode/session-1/variant/segment-1.m4s" "$transcode/.hidden" "$transcode/..dots"
	ln -s /nonexistent "$transcode/dangling"
	touch "$SYNOPKG_PKGTMP/keep" "$SYNOPKG_PKGVAR/data/library.db"

	run_sss start
	assert_success
	assert_equal "$(ls -A "$transcode")" ""
	[[ -f $SYNOPKG_PKGTMP/keep ]] || fail "a file next to the transcode directory was removed"
	[[ -f $SYNOPKG_PKGVAR/data/library.db ]] || fail "the database was removed"
}

case_start_rotates_large_log() {
	echo "previous log" >"$PKG_LOG"
	truncate -s 10485761 "$PKG_LOG"
	echo "older log" >"$PKG_LOG.1"

	run_sss start
	assert_success
	assert_equal "$(stat -c %s "$PKG_LOG.1")" 10485761
	assert_equal "$(head -n 1 "$PKG_LOG.1")" "previous log"
	refute_file_line "$PKG_LOG" "previous log"
	assert_file_line "$PKG_LOG" "fake server: starting in mode healthy"
}

case_start_keeps_log_at_rotation_limit() {
	echo "previous log" >"$PKG_LOG"
	truncate -s 10485760 "$PKG_LOG"

	run_sss start
	assert_success
	[[ ! -e $PKG_LOG.1 ]] || fail "a log of exactly 10 MiB was rotated"
	assert_equal "$(head -n 1 "$PKG_LOG")" "previous log"
}

case_start_reports_early_exit() {
	fake_mode exit-early
	local started
	started=$(monotonic_centiseconds)
	run_sss start
	assert_failure 1
	assert_elapsed_below "$started" 10 "start took too long for a server that exits"
	assert_stderr_contains "$FIXTURE_DISPLAY_NAME stopped during start (exit status 3)"
	assert_short_output
	[[ ! -e $PKG_PID ]] || fail "the PID file of the exited server is left"
	assert_equal "$(head -n 1 "$TEMP_LOGFILE")" \
		"$FIXTURE_DISPLAY_NAME stopped during start (exit status 3). Last lines of $PKG_LOG:"
	assert_equal "$(wc -l <"$TEMP_LOGFILE" | tr -d ' ')" 21
	assert_file_line "$TEMP_LOGFILE" "fake log line 11"
	assert_file_line "$TEMP_LOGFILE" "fake log line 30"
	refute_file_line "$TEMP_LOGFILE" "fake log line 10"

	run_sss status
	assert_failure 3
}

case_start_reports_early_exit_without_temp_logfile() {
	fake_mode exit-early
	unset SYNOPKG_TEMP_LOGFILE
	run_sss start
	assert_failure 1
	assert_stderr_contains "stopped during start (exit status 3)"
}

case_start_waits_for_slow_server() {
	fake_mode slow
	export FAKE_SLOW_SECONDS=2
	local started
	started=$(monotonic_centiseconds)
	run_sss start
	assert_success
	assert_output --partial "$FIXTURE_DISPLAY_NAME is running"
	assert_elapsed_at_least "$started" 2 "start returned before the server was healthy"
}

case_start_counts_alive_unhealthy_server_as_started() {
	fake_mode unhealthy
	export PKG_TEST_START_TIMEOUT_SECONDS=2
	local started
	started=$(monotonic_centiseconds)
	run_sss start
	assert_success
	assert_output --partial "not healthy after 2 s but keeps running"
	assert_elapsed_at_least "$started" 2 "start gave up before the start budget"
	assert_elapsed_below "$started" 10 "start ignored the start budget"
	process_running "$(pid_from_file)"
}

# The start budget runs on the boot clock: when it jumps past the deadline, start gives up at
# once instead of waiting the 30 s the wall clock would take.
case_start_budget_follows_boot_clock() {
	fake_mode unhealthy
	local uptime_file=$BATS_TEST_TMPDIR/uptime started
	echo "100.00 50.00" >"$uptime_file"
	export PKG_TEST_UPTIME_FILE=$uptime_file
	# The jump waits for the first health check, which comes after start read its start time.
	(
		eventually test -f "$FAKE_STATE_DIR/healthcheck.args"
		echo "1000.00 50.00" >"$uptime_file.new"
		mv "$uptime_file.new" "$uptime_file"
	) 3>&- &
	local writer=$!
	started=$(monotonic_centiseconds)
	run_sss start
	wait "$writer"
	assert_success
	assert_output --partial "not healthy after 30 s but keeps running"
	assert_elapsed_below "$started" 10 "start waited for the wall clock"
	process_running "$(pid_from_file)"
}

# A clock that stands still cannot stretch the start budget: the poll limit ends it. Leading
# zeros, octal or invalid for shell arithmetic, are read as decimal.
case_start_budget_survives_stalled_clock() {
	fake_mode unhealthy
	export PKG_TEST_START_TIMEOUT_SECONDS=02
	echo "0098.50 50.00" >"$BATS_TEST_TMPDIR/uptime"
	export PKG_TEST_UPTIME_FILE=$BATS_TEST_TMPDIR/uptime
	local started
	started=$(monotonic_centiseconds)
	run_sss start
	assert_success
	assert_output --partial "not healthy after 2 s but keeps running"
	assert_equal "$stderr" ""
	assert_elapsed_at_least "$started" 2 "start gave up before the start budget"
	assert_elapsed_below "$started" 10 "the stalled clock stretched the start budget"
	process_running "$(pid_from_file)"
}

# Without a usable /proc/uptime the start budget falls back to the wall clock. The lower bound is
# loose on purpose: that clock is the one that drifts under Docker Desktop (helpers.bash).
case_start_budget_falls_back_to_wall_clock() {
	fake_mode unhealthy
	export PKG_TEST_START_TIMEOUT_SECONDS=2
	printf 'not-a-number 1.00\n' >"$BATS_TEST_TMPDIR/garbage-uptime"
	local uptime_file started
	for uptime_file in "$BATS_TEST_TMPDIR/missing-uptime" "$BATS_TEST_TMPDIR/garbage-uptime"; do
		export PKG_TEST_UPTIME_FILE=$uptime_file
		started=$(monotonic_centiseconds)
		run_sss start
		assert_success
		assert_output --partial "not healthy after 2 s but keeps running"
		assert_equal "$stderr" ""
		assert_elapsed_at_least "$started" 1 "start gave up long before the start budget ($uptime_file)"
		assert_elapsed_below "$started" 10 "start ignored the start budget ($uptime_file)"
		run_sss stop
		assert_success
	done
}

case_start_twice_keeps_one_server() {
	run_sss start
	assert_success
	local pid
	pid=$(pid_from_file)

	run_sss start
	assert_success
	assert_output "$FIXTURE_DISPLAY_NAME is already running (pid $pid)"
	assert_equal "$(server_starts)" 1
	assert_equal "$(pid_from_file)" "$pid"
}

case_start_keeps_transcode_output_of_running_server() {
	run_sss start
	assert_success
	local transcode=$SYNOPKG_PKGTMP/transcode
	mkdir -p "$transcode/sessions/live"
	touch "$transcode/sessions/live/segment-1.m4s"

	run_sss start
	assert_success
	assert_output --partial "already running"
	[[ -f $transcode/sessions/live/segment-1.m4s ]] || fail "a second start removed the output of the running server"
}

process_gone() {
	! process_running "$1"
}

# has_child_process succeeds while process $1 has a child process.
has_child_process() {
	local stat_file stat
	for stat_file in /proc/[0-9]*/stat; do
		read -r stat 2>/dev/null <"$stat_file" || continue
		stat=${stat##*) }
		read -r _ parent _ <<<"$stat"
		[[ $parent == "$1" ]] && return 0
	done
	return 1
}

# A log that is a FIFO holds the forked shell in the redirection, before exec, until a reader
# opens it: the window in which a concurrent status used to take the fresh PID file for stale.
case_start_records_pid_only_after_exec() {
	mkfifo "$PKG_LOG"
	local -a shell_command
	read -ra shell_command <<<"$CASE_SHELL"
	"${shell_command[@]}" "$PKG_ROOT/scripts/start-stop-status" start \
		>"$BATS_TEST_TMPDIR/start.out" 2>"$BATS_TEST_TMPDIR/start.err" 3>&- &
	local start_pid=$!
	eventually has_child_process "$start_pid"
	sleep 0.5
	for _ in 1 2 3; do
		run_sss status
		assert_failure 3
		[[ ! -e $PKG_PID ]] || fail "the PID file exists before the server binary runs"
	done

	cat "$PKG_LOG" >/dev/null 3>&- &
	track_foreign_process "$!"
	local start_status=0
	wait "$start_pid" || start_status=$?
	assert_equal "$start_status" 0
	assert_equal "$(pid_from_file)" "$(cat "$FAKE_STATE_DIR/server.starts")"
	run_sss status
	assert_success
}

# start runs with its output in files, not under `run`: a child left blocked in the FIFO would
# keep run's output pipe open and hang the test instead of failing it.
case_start_kills_child_that_never_runs_the_binary() {
	# Nobody reads the FIFO, so the forked shell never gets to exec.
	mkfifo "$PKG_LOG"
	local -a shell_command
	read -ra shell_command <<<"$CASE_SHELL"
	local started start_pid start_status=0
	started=$(monotonic_centiseconds)
	"${shell_command[@]}" "$PKG_ROOT/scripts/start-stop-status" start \
		>"$BATS_TEST_TMPDIR/start.out" 2>"$BATS_TEST_TMPDIR/start.err" 3>&- &
	start_pid=$!
	eventually_within 15 process_gone "$start_pid" || {
		kill -KILL "$start_pid" 2>/dev/null || :
		fail "start did not give up on a child that never runs the binary"
	}
	wait "$start_pid" || start_status=$?
	assert_equal "$start_status" 1
	assert_elapsed_at_least "$started" 5 "start gave up before the exec wait"
	local stderr
	stderr=$(cat "$BATS_TEST_TMPDIR/start.err")
	assert_stderr_contains "$FIXTURE_DISPLAY_NAME did not start within 5 s"
	assert_file_line "$TEMP_LOGFILE" "$(head -n 1 <<<"$stderr")"
	[[ ! -e $PKG_PID ]] || fail "a PID file names a process that never became the server"
	assert_equal "$(server_starts)" 0
}

case_start_fails_and_stops_server_when_pid_file_cannot_be_written() {
	# A directory in the way of the temporary PID file fails the write, also for root.
	mkdir "$PKG_PID.new"
	run_sss start
	assert_failure 1
	assert_stderr_contains "cannot write $PKG_PID; stopped $FIXTURE_DISPLAY_NAME again"
	assert_file_line "$TEMP_LOGFILE" "cannot write $PKG_PID; stopped $FIXTURE_DISPLAY_NAME again"
	local pid
	pid=$(cat "$FAKE_STATE_DIR/server.starts")
	eventually process_gone "$pid"
	[[ ! -e $PKG_PID ]] || fail "a PID file is left"
	run_sss status
	assert_failure 3
}

case_start_names_transcode_output_it_cannot_remove() {
	local transcode=$SYNOPKG_PKGTMP/transcode
	mkdir -p "$transcode/session-1"
	touch "$transcode/session-2"
	fake_failing_command rm '*/session-1'

	run_sss start
	assert_success
	assert_stderr_contains "cannot remove $transcode/session-1"
	assert_stderr_contains "stale transcode output is left in $transcode"
	[[ ! -e $transcode/session-2 ]] || fail "the removable entry was kept"
	process_running "$(pid_from_file)"
}

case_start_names_log_it_cannot_rotate() {
	echo "previous log" >"$PKG_LOG"
	truncate -s 10485761 "$PKG_LOG"
	fake_failing_command mv '*.log'

	run_sss start
	assert_success
	assert_stderr_contains "cannot rotate $PKG_LOG; the server keeps appending to it"
	[[ ! -e $PKG_LOG.1 ]] || fail "the log was rotated although mv failed"
	assert_file_line "$PKG_LOG" "fake server: starting in mode healthy"
	process_running "$(pid_from_file)"
}

case_start_at_boot_with_empty_package_status() {
	export SYNOPKG_PKG_STATUS=
	run_sss prestart
	assert_success
	run_sss start
	assert_success
	process_running "$(pid_from_file)"
	# DSM also leaves the status empty at shutdown.
	unset SYNOPKG_PKG_STATUS
	run_sss stop
	assert_success
}

case_start_fails_without_binary() {
	rm "$SYNOPKG_PKGDEST/bin/$FIXTURE_ID"
	run_sss start
	assert_failure 1
	assert_stderr_contains "missing or not executable"
}

# --- stop ----------------------------------------------------------------------------------

case_stop_terminates_gracefully() {
	run_sss start
	assert_success
	local pid started
	pid=$(pid_from_file)
	started=$(monotonic_centiseconds)

	run_sss stop
	assert_success
	assert_short_output
	refute_output --partial SIGKILL
	assert_elapsed_below "$started" 5 "a graceful stop took too long"
	assert_equal "$(cat "$FAKE_STATE_DIR/server.signal")" term
	eventually assert_file_line "$PKG_LOG" "fake server: stopped"
	! process_running "$pid" || fail "the server still runs"
	[[ ! -e $PKG_PID ]] || fail "the PID file is left"

	run_sss status
	assert_failure 3
}

case_stop_kills_server_that_ignores_sigterm() {
	fake_mode ignore-term
	export PKG_TEST_STOP_TIMEOUT_SECONDS=2
	run_sss start
	assert_success
	local pid started
	pid=$(pid_from_file)
	started=$(monotonic_centiseconds)

	run_sss stop
	assert_success
	assert_output --partial "did not stop within 2 s; sending SIGKILL"
	assert_short_output
	assert_elapsed_at_least "$started" 2 "SIGKILL came before the stop timeout"
	# Stop budget plus the 5 s SIGKILL wait is the most stop may take; SIGKILL ends the fake at
	# once, so stop returns well before that.
	assert_elapsed_below "$started" 7 "stop waited too long after SIGKILL"
	assert_equal "$(cat "$FAKE_STATE_DIR/server.signal")" term
	! process_running "$pid" || fail "the server survived SIGKILL"
	[[ ! -e $PKG_PID ]] || fail "the PID file is left"
}

case_stop_succeeds_when_nothing_runs() {
	run_sss stop
	assert_success
	assert_output "$FIXTURE_DISPLAY_NAME is not running"

	dead_pid >"$PKG_PID"
	run_sss stop
	assert_success
	[[ ! -e $PKG_PID ]] || fail "the stale PID file is left"
}

case_stop_leaves_process_with_reused_pid_alone() {
	start_foreign_process
	echo "$FOREIGN_PID" >"$PKG_PID"
	run_sss stop
	assert_success
	process_running "$FOREIGN_PID" || fail "stop killed a process that is not the server"
	[[ ! -e $PKG_PID ]] || fail "the stale PID file is left"
}

# --- status --------------------------------------------------------------------------------

case_status_without_pid_file_is_not_running() {
	run_sss status
	assert_failure 3
	assert_output ''
	assert_equal "$stderr" ""
}

case_status_removes_pid_file_of_dead_process() {
	dead_pid >"$PKG_PID"
	run_sss status
	assert_failure 3
	[[ ! -e $PKG_PID ]] || fail "the stale PID file is left"
}

case_status_removes_pid_file_of_reused_pid() {
	start_foreign_process
	echo "$FOREIGN_PID" >"$PKG_PID"
	run_sss status
	assert_failure 3
	[[ ! -e $PKG_PID ]] || fail "the stale PID file is left"
	process_running "$FOREIGN_PID" || fail "status touched a foreign process"
}

case_status_removes_garbage_pid_file() {
	local content
	for content in "" "not-a-pid" "-1" "12 34"; do
		printf '%s\n' "$content" >"$PKG_PID"
		run_sss status
		assert_failure 3
		[[ ! -e $PKG_PID ]] || fail "the PID file '$content' is left"
	done
}

case_status_reports_missing_binary_as_broken() {
	rm "$SYNOPKG_PKGDEST/bin/$FIXTURE_ID"
	run_sss status
	assert_failure 150
	assert_stderr_contains "missing or not executable; reinstall the package"
}

case_status_reports_non_executable_binary_as_broken() {
	chmod 0644 "$SYNOPKG_PKGDEST/bin/$FIXTURE_ID"
	run_sss status
	assert_failure 150
}

# --- other actions and the environment -----------------------------------------------------

case_prestart_prestop_and_log_succeed() {
	local action
	for action in prestart prestop; do
		run_sss "$action"
		assert_success
		assert_output ''
		assert_equal "$stderr" ""
	done
	run_sss log
	assert_success
	assert_output "$PKG_LOG"
}

case_prestart_prestop_and_log_succeed_in_a_broken_package() {
	local action
	rm "$PKG_ROOT/scripts/package.env"
	for action in prestart prestop log; do
		run_sss "$action"
		assert_success
	done
}

case_unknown_argument_fails_with_usage() {
	local argument
	for argument in restart START ""; do
		run_sss "$argument"
		assert_failure 1
		assert_output ''
		assert_stderr_contains "usage:"
		assert_stderr_contains "{start|stop|status|prestart|prestop|log}"
	done
	run_sss
	assert_failure 1
	assert_stderr_contains "usage:"
}

case_missing_package_env_marks_package_broken() {
	rm "$PKG_ROOT/scripts/package.env"
	run_sss status
	assert_failure 150
	assert_stderr_contains "package.env is missing"
	run_sss start
	assert_failure 1
	assert_equal "$(server_starts)" 0
}

case_invalid_package_env_marks_package_broken() {
	local line
	for line in "ENV_PREFIX='demo'" "ENV_PREFIX='DEMO-X'" "ID='../demo'" "ID=''" "ID='de_mo'" \
		"ID='Demo'" "DISPLAY_NAME=''"; do
		cp "$TEST_DIR/fixtures/package.env" "$PKG_ROOT/scripts/package.env"
		echo "$line" >>"$PKG_ROOT/scripts/package.env"
		run_sss status
		assert_failure 150
		assert_stderr_contains "package.env:"
	done
}

case_missing_common_sh_marks_package_broken() {
	rm "$PKG_ROOT/scripts/common.sh"
	run_sss status
	assert_failure 150
	assert_stderr_contains "common.sh is missing"
	run_sss start
	assert_failure 1
}

case_missing_dsm_variable_is_unknown() {
	local variable
	for variable in SYNOPKG_PKGDEST SYNOPKG_PKGVAR SYNOPKG_PKGTMP SYNOPKG_PKGHOME; do
		(
			unset "$variable"
			run_sss status
			assert_failure 4
			assert_stderr_contains "$variable is not set"
			run_sss start
			assert_failure 1
		)
	done
	export SYNOPKG_PKGVAR=relative/var
	run_sss status
	assert_failure 4
	assert_stderr_contains "SYNOPKG_PKGVAR is not an absolute path"
	assert_equal "$(server_starts)" 0
}

case_scripts_find_common_sh_from_any_directory() {
	cd "$BATS_TEST_TMPDIR" || fail "cannot enter $BATS_TEST_TMPDIR"
	run_script "packages/$FIXTURE_ID/scripts/start-stop-status" status
	assert_failure 3

	cd "$PKG_ROOT/scripts" || fail "cannot enter the scripts directory"
	run_script start-stop-status status
	assert_failure 3

	# On DSM /var/packages/<id> is reached through links as well.
	ln -s "$PKG_ROOT/scripts" "$BATS_TEST_TMPDIR/linked-scripts"
	cd / || fail "cannot enter /"
	run_script "$BATS_TEST_TMPDIR/linked-scripts/start-stop-status" start
	assert_success
	run_script "$BATS_TEST_TMPDIR/linked-scripts/start-stop-status" status
	assert_success
}

# spkstage writes package.env from spkmeta's Package.PackageEnv: the assignments only, without
# the fixture's comments. TestScriptFixtureIsSpkmetaOutput (packaging/synology/cmd/spkstage)
# proves that the fixture's assignments are exactly spkmeta's output for its brand.
case_spkmeta_package_env_works() {
	grep -v '^#' "$TEST_DIR/fixtures/package.env" >"$PKG_ROOT/scripts/package.env"
	run_sss start
	assert_success
	assert_file_line "$FAKE_STATE_DIR/server.env" "DEMO_CONFIG_DIR=$SYNOPKG_PKGVAR"
}

register_cases \
	start_healthy_server \
	start_exports_package_environment \
	start_sends_server_output_to_its_log_only \
	start_creates_server_directories \
	start_removes_stale_transcode_output \
	start_rotates_large_log \
	start_keeps_log_at_rotation_limit \
	start_reports_early_exit \
	start_reports_early_exit_without_temp_logfile \
	start_waits_for_slow_server \
	start_counts_alive_unhealthy_server_as_started \
	start_budget_follows_boot_clock \
	start_budget_survives_stalled_clock \
	start_budget_falls_back_to_wall_clock \
	start_twice_keeps_one_server \
	start_keeps_transcode_output_of_running_server \
	start_records_pid_only_after_exec \
	start_kills_child_that_never_runs_the_binary \
	start_fails_and_stops_server_when_pid_file_cannot_be_written \
	start_names_transcode_output_it_cannot_remove \
	start_names_log_it_cannot_rotate \
	start_at_boot_with_empty_package_status \
	start_fails_without_binary \
	stop_terminates_gracefully \
	stop_kills_server_that_ignores_sigterm \
	stop_succeeds_when_nothing_runs \
	stop_leaves_process_with_reused_pid_alone \
	status_without_pid_file_is_not_running \
	status_removes_pid_file_of_dead_process \
	status_removes_pid_file_of_reused_pid \
	status_removes_garbage_pid_file \
	status_reports_missing_binary_as_broken \
	status_reports_non_executable_binary_as_broken \
	prestart_prestop_and_log_succeed \
	prestart_prestop_and_log_succeed_in_a_broken_package \
	unknown_argument_fails_with_usage \
	missing_package_env_marks_package_broken \
	invalid_package_env_marks_package_broken \
	missing_common_sh_marks_package_broken \
	missing_dsm_variable_is_unknown \
	scripts_find_common_sh_from_any_directory \
	spkmeta_package_env_works

# --- static checks, independent of the shell -----------------------------------------------

@test "package files do not name the product" {
	local product_id
	product_id=$(sed -n 's/^[[:space:]]*ID = "\(.*\)"$/\1/p' "$REPO_ROOT/internal/platform/brand/brand.go")
	[[ -n $product_id ]] || fail "brand.ID not found in internal/platform/brand/brand.go"
	run grep -rli -- "$product_id" "$REPO_ROOT/packaging/synology/package"
	assert_failure 1
	assert_output ''
}

@test "package scripts and test files use LF line endings" {
	run grep -rl $'\r' "$PACKAGE_SCRIPTS" "$TEST_DIR"
	assert_failure 1
	assert_output ''
}

@test "package.env is generated, not a template, and scripts hold no build tokens" {
	# spkstage writes package.env from spkmeta (one source of truth); a template next to it would
	# be a second one, and spkstage refuses to stage both.
	[[ ! -e $PACKAGE_SCRIPTS/package.env ]] || fail "$PACKAGE_SCRIPTS/package.env must not exist"
	run grep -o '@[A-Z_]*@' -r "$PACKAGE_SCRIPTS"
	assert_failure 1
}

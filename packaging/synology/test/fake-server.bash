# shellcheck shell=bash
# Fake media server for the package script tests (README.md in this directory).
#
# In a test the package binary bin/<id> is a copy of the bash executable, so /proc/<pid>/exe of
# the running fake names the package binary exactly as it does for the real server. bash reads
# this file through BASH_ENV before anything else and the file always exits, so bash never gets
# to its own argument handling. Every other bash process that inherits BASH_ENV (the script
# under test when it runs under bash) returns at the guard below.
#
# Invocations, like the real binary:
#   <id>               serve until SIGTERM (bash: $0 is the binary path)
#   <id> healthcheck   exit 0 when the control file "health" says ok (bash: $0 is "healthcheck")
#
# Control files in $FAKE_STATE_DIR, written by the tests:
#   mode    healthy (default) | slow | unhealthy | exit-early | ignore-term
#   health  "ok" makes the healthcheck succeed; healthy and ignore-term write it at start,
#           slow after FAKE_SLOW_SECONDS (default 2), unhealthy and exit-early never
# Records in $FAKE_STATE_DIR, read by the tests:
#   server.starts                    one PID per server start
#   server.args, server.env          arguments (one per line) and sorted environment
#   healthcheck.args, healthcheck.env  the same for the last healthcheck
#   server.signal                    "term" once SIGTERM arrived

[[ ${BASH##*/} == "${FAKE_BINARY_NAME:-}" ]] || return 0

fake_record() {
	local name=$1
	shift
	printf '%s\n' "$@" >"$FAKE_STATE_DIR/$name.args"
	env | sort >"$FAKE_STATE_DIR/$name.env"
}

fake_healthcheck() {
	fake_record healthcheck "$@"
	if [[ $(cat "$FAKE_STATE_DIR/health" 2>/dev/null) == ok ]]; then
		exit 0
	fi
	echo "fake healthcheck: not healthy" >&2
	exit 1
}

fake_exit_early() {
	local line
	sleep 0.3
	for ((line = 1; line <= 30; line++)); do
		echo "fake log line $line"
	done
	exit 3
}

# fake_uptime_seconds prints the whole seconds since boot. slow mode measures with it, not with
# SECONDS, for the reason given at monotonic_centiseconds in scripts/helpers.bash.
fake_uptime_seconds() {
	local uptime rest
	read -r uptime rest </proc/uptime
	echo "${uptime%%.*}"
}

fake_serve() {
	local mode healthy_at
	mode=$(cat "$FAKE_STATE_DIR/mode" 2>/dev/null || echo healthy)
	# One second more than asked, because the start is rounded down.
	healthy_at=$(($(fake_uptime_seconds) + ${FAKE_SLOW_SECONDS:-2} + 1))
	echo "$$" >>"$FAKE_STATE_DIR/server.starts"
	fake_record server
	echo "fake server: starting in mode $mode"
	echo "fake server: diagnostics on stderr" >&2
	case $mode in
	exit-early) fake_exit_early ;;
	ignore-term) trap 'echo term >"$FAKE_STATE_DIR/server.signal"' TERM ;;
	*) trap 'echo term >"$FAKE_STATE_DIR/server.signal"; echo "fake server: stopped"; exit 0' TERM ;;
	esac
	if [[ $mode == healthy || $mode == ignore-term ]]; then
		echo ok >"$FAKE_STATE_DIR/health"
	fi
	while :; do
		if [[ $mode == slow ]] && (($(fake_uptime_seconds) >= healthy_at)); then
			echo ok >"$FAKE_STATE_DIR/health"
		fi
		# A background sleep keeps the shell able to run the TERM trap at once.
		sleep 0.1 &
		wait "$!" || :
	done
}

if [[ $0 == "$BASH" ]]; then
	fake_serve
elif [[ $0 == healthcheck ]]; then
	fake_healthcheck "$@"
fi
fake_record unexpected "$0" "$@"
echo "fake server: unexpected arguments: $0 $*" >&2
exit 2

#!/usr/bin/env bash
# smoke.sh - emulated install and playback smoke of one server SPK (task SPK-11).
#
#   smoke.sh [options] <family> <file.spk> [cpu-model ...]
#
#   family        x86_64 (linux/amd64), armv8 (linux/arm64) or armv7 (linux/arm/v7)
#   cpu-model     QEMU CPU models, one complete run each (QEMU_CPU accepts properties such as
#                 "cortex-a9,neon=off,vfp-d32=off"). Defaults: armv8 cortex-a53, armv7
#                 cortex-a9, cortex-a15 and cortex-a9,neon=off,vfp-d32=off (the Armada 38x worst
#                 case). x86_64 runs natively ("native"; README.md says why).
#   --host-port N           host port 127.0.0.1:N for the package's adminport (default 8460)
#   --share NAME            name of the shared folder that holds the corpus (default video)
#   --case-timeout D        -case-timeout of playback-smoke's first run (default: its 2m)
#   --slow-case-timeout D   -case-timeout and -scan-timeout of the rerun of a profile whose
#                           only failures are timeouts (default 15m; also
#                           SMOKE_SLOW_CASE_TIMEOUT). Any other failure is a FAIL.
#   --ffprobe PATH          local ffprobe for playback-smoke (default: ffprobe on PATH)
#   --work DIR              directory for the logs (also SMOKE_WORK; default: a new temporary
#                           directory); it is kept and named at the end
#   --keep                  leave the container running after the run (debugging)
#   -h, --help              this text
#
# Per CPU model: a debian:stable-slim container of the family's platform (Dockerfile here; QEMU
# through Docker Desktop's binfmt for arm) gets the SPK, the corpus testdata/media read-only as
# /volume1/<share>, kernelsim (built from ../../ffmpeg/build/verify) and dsm.sh, which installs
# the SPK into a DSM-like layout and runs every package script as the package user under the
# syscall list of the family's oldest DSM 7.2 kernel. The run: inspect-spk.sh --release (once
# per family), install, versions, prestart + start, status, scripts/playback-smoke with both
# reference profiles against the published port, stop within 20 s, the checks after the stop,
# upgrade (same SPK) with start and a check that the database was kept, uninstall keeping the
# data, reinstall with start, uninstall deleting the data. Exit status 0 when everything
# passed, 1 when anything failed, 2 on a usage error.
#
# Needs bash >= 4, Docker on an x86_64 (amd64) host - checked; the tools and the x86_64 run are
# native there - (Docker Desktop on Windows, or Linux with binfmt QEMU for arm64 and arm/v7), Go (builds scripts/playback-smoke), curl, ffprobe and the corpus
# (go run ./scripts/gen-testmedia). Runs from Git Bash on Windows and from Linux shells.
set -euo pipefail

# Git Bash must not rewrite container paths such as /smoke into Windows paths.
export MSYS_NO_PATHCONV=1

readonly PROFILES=(chromium-desktop firefox-desktop)
readonly DEFAULT_SHARE=video
readonly DEFAULT_HOST_PORT=8460
readonly DEFAULT_SLOW_TIMEOUT=15m
# playback-smoke's own -case-timeout default (scripts/playback-smoke/main.go).
readonly DEFAULT_CASE_TIMEOUT=2m
readonly INSPECT_IMAGE=spk-inspect:dev
readonly SMOKE_IMAGE_PREFIX=spk-smoke
# QEMU model of the armv7 worst case: Cortex-A9 with VFPv3-D16 and without NEON.
readonly ARMV7_WORST_CASE_CPU="cortex-a9,neon=off,vfp-d32=off"
# Baseline CPU of the x86_64 version commands (the model verify.sh uses).
readonly X86_BASELINE_CPU=Opteron_G1
# Seconds to wait for the scan the server starts after an upgrade or reinstall.
readonly STARTUP_SCAN_WAIT_SECONDS=900
# Seconds to wait for the API on the published port after start.
readonly PORT_WAIT_SECONDS=180
readonly CORPUS_MANIFEST=testdata/media/manifest.json
# Throwaway administrator of the package under test (sign-in is on since M7-25).
readonly SMOKE_ACCOUNT=smoke

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly script_dir
repo="$(cd "$script_dir/../../../.." && pwd)"
readonly repo
readonly verify_dir="$repo/packaging/synology/ffmpeg/build/verify"

host_port=$DEFAULT_HOST_PORT
share=$DEFAULT_SHARE
case_timeout=""
slow_timeout="${SMOKE_SLOW_CASE_TIMEOUT:-$DEFAULT_SLOW_TIMEOUT}"
ffprobe=ffprobe
work="${SMOKE_WORK:-}"
keep=no
# 128 random bits per run, hex; held in memory only and never logged.
smoke_password=$(head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n')
session_token=""
results=()
failed=0

log() { printf '[smoke] %s\n' "$*"; }
die() {
	printf '[smoke] error: %s\n' "$*" >&2
	exit 1
}

usage() {
	sed -n '2,/^set -euo pipefail$/{/^set /d;s/^# \{0,1\}//;p}' "${BASH_SOURCE[0]}"
	exit "${1:-0}"
}

# host_path converts a path for docker -v and native Windows programs (C:/... form).
host_path() {
	if command -v cygpath >/dev/null 2>&1; then cygpath -m "$1"; else printf '%s' "$1"; fi
}

# record adds a result line ("PASS ...", "FAIL ..." or "INFO ...") and notes a failure.
record() {
	results+=("$1 $2")
	printf '[smoke] %s %s\n' "$1" "$2"
	[ "$1" != FAIL ] || failed=1
}

parse_args() {
	local positional=()
	while [ "$#" -gt 0 ]; do
		case "$1" in
		--host-port | --share | --case-timeout | --slow-case-timeout | --ffprobe | --work)
			[ "$#" -ge 2 ] && [ -n "$2" ] || { log "$1 needs a value" && usage 2; }
			case "$1" in
			--host-port) host_port=$2 ;;
			--share) share=$2 ;;
			--case-timeout) case_timeout=$2 ;;
			--slow-case-timeout) slow_timeout=$2 ;;
			--ffprobe) ffprobe=$2 ;;
			--work) work=$2 ;;
			esac
			shift
			;;
		--keep) keep=yes ;;
		-h | --help) usage 0 ;;
		-*) log "unknown option $1" && usage 2 ;;
		*) positional+=("$1") ;;
		esac
		shift
	done
	[ "${#positional[@]}" -ge 2 ] || usage 2
	family=${positional[0]}
	spk=${positional[1]}
	cpus=("${positional[@]:2}")
	[[ "$host_port" =~ ^[1-9][0-9]{0,4}$ ]] || { log "--host-port must be a port number" && usage 2; }
	[[ "$share" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] || { log "--share takes a plain folder name" && usage 2; }
}

# load_family sets the platform, the oldest DSM 7.2 kernel and syscall list (ADR 0026 Notes),
# a DSM platform name for SYNOPKG_DSM_ARCH and the default CPU models.
load_family() {
	case "$family" in
	x86_64) platform=linux/amd64 kernel=3.10.108 syscalls=syscalls-after-3.10.txt default_cpus=(native) ;;
	armv8) platform=linux/arm64 kernel=4.4.302 syscalls=syscalls-after-4.4.txt default_cpus=(cortex-a53) ;;
	# cortex-a9 without NEON and with 16 double registers is the Armada 38x worst case of
	# ADR 0026 Notes (VFPv3-D16, NEON only through run-time detection).
	armv7) platform=linux/arm/v7 kernel=3.10.108 syscalls=syscalls-after-3.10.txt
		default_cpus=(cortex-a9 cortex-a15 "$ARMV7_WORST_CASE_CPU") ;;
	*) log "unknown family $family" && usage 2 ;;
	esac
	[ "${#cpus[@]}" -gt 0 ] || cpus=("${default_cpus[@]}")
	if [ "$family" = x86_64 ] && [ "${cpus[*]}" != native ]; then
		log "x86_64 runs natively only (README.md); CPU models are not supported" && usage 2
	fi
}

# dsm_arch prints a DSM platform of the family that matches the CPU model.
dsm_arch() {
	case "$family:$1" in
	x86_64:*) echo v1000 ;;
	armv8:*) echo rtd1619b ;;
	armv7:cortex-a15*) echo alpine ;;
	armv7:*) echo armada38x ;;
	esac
}

# info_value prints a value of the SPK's INFO (lines key="value").
info_value() { tar -xOf "$spk" INFO | sed -n "s/^$1=\"\(.*\)\"\$/\1/p"; }

prepare() {
	command -v docker >/dev/null 2>&1 || die "docker not found"
	command -v go >/dev/null 2>&1 || die "go not found"
	command -v curl >/dev/null 2>&1 || die "curl not found"
	# kernelsim, sysprobe and qemu-x86_64 are x86_64 binaries that run natively in every
	# container, and x86_64 runs without emulation: the Docker host must be x86_64.
	local docker_architecture
	docker_architecture=$(docker info --format '{{.Architecture}}') || die "docker info failed"
	[ "$docker_architecture" = x86_64 ] || die "the Docker host must be x86_64 (amd64), not '$docker_architecture'"
	[ -f "$spk" ] || die "$spk not found"
	spk=$(cd "$(dirname "$spk")" && pwd)/$(basename "$spk")
	[ -f "$repo/$CORPUS_MANIFEST" ] || die "$CORPUS_MANIFEST is missing; run go run ./scripts/gen-testmedia"
	package_id=$(info_value package)
	admin_port=$(info_value adminport)
	[ -n "$package_id" ] && [ -n "$admin_port" ] || die "$spk has no package or adminport in INFO"
	[ -n "$work" ] || work=$(mktemp -d "${TMPDIR:-/tmp}/spk-smoke-$family.XXXXXX")
	mkdir -p "$work"
	work=$(cd "$work" && pwd)
	log "family $family ($platform), SPK $(basename "$spk") ($(wc -c <"$spk") bytes), package $package_id, CPU models: ${cpus[*]}"
	log "work directory $work"
}

inspect_spk() {
	log "inspect-spk.sh --release"
	docker build -q -t "$INSPECT_IMAGE" "$(host_path "$script_dir/../inspect")" >/dev/null
	if docker run --rm -v "$(host_path "$(dirname "$spk")"):/in:ro" -v "$(host_path "$repo"):/code:ro" -w /code \
		"$INSPECT_IMAGE" sh packaging/synology/test/inspect/inspect-spk.sh --release "/in/$(basename "$spk")" "$family" \
		>"$work/inspect.log" 2>&1; then
		record PASS "$family inspect-spk.sh --release: $(tail -n 1 "$work/inspect.log")"
		return 0
	fi
	grep -E '^(FAIL|WARN)' "$work/inspect.log" || :
	record FAIL "$family inspect-spk.sh --release: $(tail -n 1 "$work/inspect.log")"
	return 1
}

build_tools() {
	log "building kernelsim, sysprobe and qemu-x86_64 (ffmpeg/build/verify), the $platform image and playback-smoke"
	docker build --quiet --output "type=local,dest=$(host_path "$work/tools")" "$(host_path "$verify_dir")" >/dev/null
	smoke_image="$SMOKE_IMAGE_PREFIX:$family"
	docker build --quiet --platform "$platform" -t "$smoke_image" "$(host_path "$script_dir")" >/dev/null
	smoke_binary="$work/playback-smoke$(go env GOEXE)"
	(cd "$repo" && go build -o "$(host_path "$smoke_binary")" ./scripts/playback-smoke)
}

# start_container starts the idle container of one CPU model.
start_container() {
	local cpu=$1 emulation=()
	if [ "$cpu" = native ]; then
		emulation=(-e SMOKE_EMULATED=no)
	else
		emulation=(-e SMOKE_EMULATED=yes -e "QEMU_CPU=$cpu" -e "QEMU_UNAME=$kernel")
	fi
	docker rm -f "$container" >/dev/null 2>&1 || :
	docker run -d --init --name "$container" --platform "$platform" \
		-p "127.0.0.1:$host_port:$admin_port" \
		-v "$(host_path "$spk"):/input/package.spk:ro" \
		-v "$(host_path "$repo/testdata/media"):/volume1/$share:ro" \
		-v "$(host_path "$work/tools"):/tools:ro" -v "$(host_path "$verify_dir"):/verify:ro" \
		-v "$(host_path "$script_dir"):/smoke:ro" \
		-e "SMOKE_SYSCALLS=/verify/$syscalls" -e "SMOKE_FAMILY=$family" -e "SMOKE_KERNEL=$kernel" \
		-e "SMOKE_X86_CPU=$X86_BASELINE_CPU" -e "SMOKE_DSM_ARCH=$(dsm_arch "$cpu")" \
		"${emulation[@]}" "$smoke_image" sleep infinity >/dev/null
}

# remove_container removes the container of the current CPU model unless --keep; it runs from
# the EXIT, INT and TERM traps too, so an aborted run leaves no container behind.
remove_container() {
	[ -n "${container:-}" ] || return 0
	if [ "$keep" = yes ]; then
		log "container $container kept running"
	else
		docker rm -f "$container" >/dev/null 2>&1 || :
	fi
	container=""
}

# shellcheck disable=SC2329 # invoked by the INT and TERM trap
on_signal() {
	log "interrupted"
	remove_container
	exit 130
}

# dsm runs one dsm.sh command in the container and appends its output to the run log.
dsm() {
	docker exec "$container" /bin/sh /smoke/dsm.sh "$@" 2>&1 | tee -a "$run_log"
	return "${PIPESTATUS[0]}"
}

# step <description> <dsm.sh arguments...> records PASS or FAIL for one dsm.sh command.
step() {
	local description=$1 started=$SECONDS
	shift
	if dsm "$@"; then
		record PASS "$label $description ($((SECONDS - started)) s)"
	else
		record FAIL "$label $description ($((SECONDS - started)) s)"
		return 1
	fi
}

# open_api calls a route that needs no sign-in (system/info, setup state).
open_api() { curl -fsS --max-time 30 "http://127.0.0.1:$host_port/api/v1/$1"; }

# api calls a protected route with the session token of authenticate. The token goes to curl
# through a config on stdin, so it shows up neither in the process list nor on disk.
api() {
	printf 'header = "Authorization: Bearer %s"\n' "$session_token" |
		curl -fsS --max-time 30 -K - "http://127.0.0.1:$host_port/api/v1/$1"
}

# json_post <path> <json on stdin> posts a body that may hold the password without putting it
# into an argument.
json_post() {
	curl -fsS --max-time 30 -H 'Content-Type: application/json' --data-binary @- \
		"http://127.0.0.1:$host_port/api/v1/$1"
}

# authenticate gets a bearer token (client cli) for the throwaway account: the first-run setup
# while the server has no account (LAN-only, ADR 0032 section 6), else a sign-in, which is what
# a restart on a kept data directory needs. The password is generated per run and exists only in
# this shell and the environment of playback-smoke; neither it nor the token is ever printed.
authenticate() {
	local state body response path=auth/session what="sign-in"
	state=$(open_api setup) || { record FAIL "$label GET /api/v1/setup failed"; return 1; }
	if [[ "$state" == *'"required":true'* ]]; then path=setup what="first-run setup"; fi
	body=$(printf '{"name":"%s","password":"%s","client":"cli","deviceName":"smoke.sh"}' "$SMOKE_ACCOUNT" "$smoke_password")
	if ! response=$(printf '%s' "$body" | json_post "$path" 2>/dev/null); then
		record FAIL "$label $what as $SMOKE_ACCOUNT through POST /api/v1/$path failed"
		return 1
	fi
	session_token=$(sed -n 's/.*"token":"\([^"]*\)".*/\1/p' <<<"$response")
	if [ -z "$session_token" ]; then
		record FAIL "$label $what answered without a bearer token"
		return 1
	fi
	record PASS "$label $what as $SMOKE_ACCOUNT, bearer token obtained"
}

# check_published_port waits until the server answers on the published port: start counts a
# server that is alive but not yet healthy after its 30 s budget as started, which is likely
# under emulation. It ends with authenticate, as every later API call needs the session.
check_published_port() {
	local info deadline=$((SECONDS + PORT_WAIT_SECONDS))
	until info=$(open_api system/info 2>/dev/null); do
		if [ "$SECONDS" -ge "$deadline" ]; then
			record FAIL "$label port $admin_port is not reachable as 127.0.0.1:$host_port after $PORT_WAIT_SECONDS s"
			return 1
		fi
		sleep 1
	done
	record PASS "$label port $admin_port published as 127.0.0.1:$host_port: $info"
	authenticate
}

# non_timeout_failures prints the FAIL lines of the playback-smoke log $1 that are not timeouts
# of a case limited to the Go duration $2: a timeout either reports "deadline exceeded" or, when
# it cut a download or probe short, shows up as another error of a case that ran at least $2
# (the duration in parentheses that ends every case line).
non_timeout_failures() {
	grep '^FAIL ' "$1" | grep -v 'deadline exceeded' | awk -v limit="$2" '
		function seconds(duration, total, part, number, unit) {
			total = 0
			while (match(duration, /^[0-9.]+(h|ms|m|s|us|ns|\302\265s)/)) {
				part = substr(duration, 1, RLENGTH)
				duration = substr(duration, RLENGTH + 1)
				match(part, /^[0-9.]+/)
				number = substr(part, 1, RLENGTH) + 0
				unit = substr(part, RLENGTH + 1)
				total += number * (unit == "h" ? 3600 : unit == "m" ? 60 : unit == "s" ? 1 : unit == "ms" ? 0.001 : unit == "ns" ? 1e-9 : 1e-6)
			}
			return total
		}
		{
			elapsed = $NF
			gsub(/[()]/, "", elapsed)
			if (seconds(elapsed) < seconds(limit)) print
		}' || :
}

# timeouts_only succeeds when the playback-smoke log $1 of a run with case timeout $2 failed
# by timeouts only: case timeouts (non_timeout_failures) or the scan timeout, and no other error.
timeouts_only() {
	local failures others
	failures=$(grep -c '^FAIL ' "$1" || :)
	others=$(grep '^playback-smoke: ' "$1" | grep -v 'at least one case did not play' | grep -vc 'deadline exceeded' || :)
	[ -z "$(non_timeout_failures "$1" "$2")" ] && [ "$others" -eq 0 ] &&
		{ [ "$failures" -gt 0 ] || grep -q '^playback-smoke: .*deadline exceeded' "$1"; }
}

# run_playback runs playback-smoke for profile $1 with the extra options $2..., logs to $playback_log.
run_playback() {
	local profile=$1
	shift
	# playback-smoke signs in as the throwaway account itself; the password reaches it only
	# through the environment variable.
	(cd "$repo" && MIVO_SMOKE_PASSWORD="$smoke_password" "$smoke_binary" -user "$SMOKE_ACCOUNT" \
		-password-env MIVO_SMOKE_PASSWORD -base "http://127.0.0.1:$host_port" -manifest "$CORPUS_MANIFEST" \
		-profile "$profile" -ffprobe "$ffprobe" "$@") 2>&1 | tee "$playback_log"
	return "${PIPESTATUS[0]}"
}

# smoke_profile runs playback-smoke with one profile; a run that failed only by timeouts is
# repeated with the slow timeouts and both durations are reported.
smoke_profile() {
	local profile=$1 started=$SECONDS options=() scan summary
	playback_log="$run_dir/playback-$profile.log"
	[ -z "$case_timeout" ] || options=(-case-timeout "$case_timeout")
	if run_playback "$profile" "${options[@]}"; then
		scan=$(sed -n 's/^scanned .*(\(.*\))$/\1/p' "$playback_log")
		summary=$(grep '^summary:' "$playback_log")
		record PASS "$label playback-smoke $profile: scan ${scan:-?}, $summary (wall $((SECONDS - started)) s)"
		return 0
	fi
	if ! timeouts_only "$playback_log" "${case_timeout:-$DEFAULT_CASE_TIMEOUT}"; then
		record FAIL "$label playback-smoke $profile: $(grep -E '^(summary:|playback-smoke:)' "$playback_log" | tr '\n' ' ')"
		return 1
	fi
	log "$profile failed by timeouts only after $((SECONDS - started)) s; rerun with -case-timeout and -scan-timeout $slow_timeout"
	cp "$playback_log" "$run_dir/playback-$profile-timeouts.log"
	started=$SECONDS
	if run_playback "$profile" -case-timeout "$slow_timeout" -scan-timeout "$slow_timeout"; then
		scan=$(sed -n 's/^scanned .*(\(.*\))$/\1/p' "$playback_log")
		record PASS "$label playback-smoke $profile (rerun, timeouts $slow_timeout): scan ${scan:-?}, $(grep '^summary:' "$playback_log") (wall $((SECONDS - started)) s)"
		return 0
	fi
	record FAIL "$label playback-smoke $profile (rerun, timeouts $slow_timeout): $(grep -E '^(summary:|playback-smoke:)' "$playback_log" | tr '\n' ' ')"
	return 1
}

# check_kept_catalogue waits for the scan the server starts on its own and passes when it found
# every file unchanged: the database of the previous installation was used.
check_kept_catalogue() {
	local what=$1 deadline=$((SECONDS + STARTUP_SCAN_WAIT_SECONDS)) libraries=""
	# The data directory was kept, so the account exists: this signs in instead of setting up.
	authenticate || return 1
	while [ "$SECONDS" -lt "$deadline" ]; do
		libraries=$(api libraries || :)
		if [[ "$libraries" == *'"state":"failed"'* ]]; then
			record FAIL "$label $what: the scan after the start failed: $libraries"
			return 1
		fi
		if [[ "$libraries" == *'"state":"idle"'* && "$libraries" == *'"finishedAt"'* ]]; then break; fi
		sleep 2
	done
	# Files outnumber items: extras (trailers, featurettes) and the parts of one movie are files
	# of the same item. A kept catalogue therefore means no file added or failed and at least one
	# unchanged file per item.
	local items unchanged added failed
	items=$(sed -n 's/.*"itemCount":\([0-9]*\).*/\1/p' <<<"$libraries")
	unchanged=$(sed -n 's/.*"unchanged":\([0-9]*\).*/\1/p' <<<"$libraries")
	added=$(sed -n 's/.*"added":\([0-9]*\).*/\1/p' <<<"$libraries")
	failed=$(sed -n 's/.*"failed":\([0-9]*\).*/\1/p' <<<"$libraries")
	if [ -n "$items" ] && [ "${items:-0}" -gt 0 ] && [ "$added" = 0 ] && [ "${failed:-0}" = 0 ] &&
		[ -n "$unchanged" ] && [ "$unchanged" -ge "$items" ]; then
		record PASS "$label $what: catalogue kept ($items items, scan after start: $unchanged files unchanged, 0 added, 0 failed)"
	else
		record FAIL "$label $what: catalogue not kept or scan unfinished: ${libraries:-no answer}"
		return 1
	fi
}

# install_and_start installs the SPK and starts it the way Package Center does after an
# installation with "Run after installation"; it fails at the first failed step.
install_and_start() {
	step "share snapshot" snapshot "$share" &&
		step "install (preinst, postinst, resources)" install /input/package.spk &&
		step "probe (uname, hwcap, syscall filter)" probe &&
		step "versions (<id> --version, ffmpeg -version, ffprobe -version)" versions &&
		step "prestart" control INSTALL prestart &&
		step "start" control INSTALL start &&
		step "status exits 0" control START status &&
		check_published_port
}

api_post() {
	printf 'header = "Authorization: Bearer %s"\n' "$session_token" |
		curl -fsS --max-time 30 -K - -H 'Content-Type: application/json' -d "$2" "http://127.0.0.1:$host_port/api/v1/$1"
}

# check_first_start_state passes when a fresh installation has zero libraries and the folder
# browser starts at the volume (install kind synology, browse root /volume1): the state in which
# the web app shows its empty state with "Add library". Sets volume_root_id.
check_first_start_state() {
	local libraries roots
	libraries=$(api libraries) || { record FAIL "$label GET /api/v1/libraries failed"; return 1; }
	if [ "$libraries" != '{"libraries":[]}' ]; then
		record FAIL "$label a fresh installation has libraries: $libraries"
		return 1
	fi
	roots=$(api folder-roots) || { record FAIL "$label GET /api/v1/folder-roots failed"; return 1; }
	volume_root_id=$(grep -o '{[^{}]*"path":"/volume1"[^{}]*}' <<<"$roots" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p' | head -n 1)
	if [ -z "$volume_root_id" ] || [[ "$roots" != *'"installKind":"synology"'* ]]; then
		record FAIL "$label folder roots lack /volume1 or installKind synology: $roots"
		return 1
	fi
	record PASS "$label first start: GET /api/v1/libraries is empty, folder root /volume1 ($volume_root_id), installKind synology"
}

# add_library_through_the_api creates the library of the corpus share below the browse root
# /volume1, as the web app's folder browser does; the server scans it after creating it.
add_library_through_the_api() {
	local body response
	body="{\"kind\":\"movies\",\"name\":\"Smoke\",\"folders\":[{\"rootId\":\"$volume_root_id\",\"path\":\"$share\"}]}"
	if ! response=$(api_post libraries "$body"); then
		record FAIL "$label POST /api/v1/libraries for /volume1/$share failed"
		return 1
	fi
	record PASS "$label library added through POST /api/v1/libraries: $(cut -c1-200 <<<"$response")"
}

# stop_and_check stops the server as Package Center's "Stop" does and checks what is left.
stop_and_check() {
	step "stop within 20 s" stop STOP || :
	step "after the stop: status 3, no process, sessions directory empty, share unchanged, no log error" \
		check-stopped "$share" played || :
}

# lifecycle runs the upgrade (to the same SPK), the uninstall that keeps the data, the
# reinstall that finds it and the uninstall that deletes it.
lifecycle() {
	step "start before the upgrade" control START start &&
		step "upgrade (stop, preupgrade, preuninst, postuninst, preinst, postinst, postupgrade)" upgrade /input/package.spk &&
		step "prestart after the upgrade" control UPGRADE prestart &&
		step "start after the upgrade" control UPGRADE start &&
		step "status exits 0 after the upgrade" control START status &&
		check_kept_catalogue "upgrade" &&
		step "collect logs" collect &&
		step "uninstall keeping the data" uninstall false &&
		step "reinstall (keeps var, with the library in the database)" install /input/package.spk &&
		step "start after the reinstall" control INSTALL start &&
		check_kept_catalogue "reinstall" &&
		step "stop after the reinstall" stop STOP &&
		step "after the stop: checks" check-stopped "$share" &&
		step "collect logs" collect &&
		step "uninstall deleting the data" uninstall true
}

# finish_run copies the logs out of the container and removes it unless --keep.
finish_run() {
	docker cp "$container:/var/tmp/smoke-state/logs" "$(host_path "$run_dir/logs")" >/dev/null 2>&1 || :
	docker cp "$container:/var/log/packages" "$(host_path "$run_dir/var-log-packages")" >/dev/null 2>&1 || :
	remove_container
}

# uname_note describes the kernel the run's processes see.
uname_note() {
	if [ "$1" = native ]; then
		echo "uname host, version commands $kernel"
	else
		echo "uname $kernel"
	fi
}

# run_alpha2_upgrade is the second scenario of a CPU model, in a fresh container: an installation
# shaped like 0.1.0-alpha.2's (its config.yaml with the share link as library path, no database
# yet) is upgraded to this SPK; the library must then be listed with the path /volume1/<share>
# and be scanned.
run_alpha2_upgrade() {
	local cpu=$1 libraries
	label="$family/$cpu alpha.2 upgrade"
	run_log="$run_dir/dsm-alpha2.log"
	container="spk-smoke-$family-${cpu//[^A-Za-z0-9_.-]/-}-alpha2"
	start_container "$cpu"
	if step "install" install /input/package.spk &&
		step "seed an alpha.2-shaped config.yaml and share link" seed-alpha2 "$share" &&
		step "upgrade (preupgrade records the link, postupgrade rewrites the path)" upgrade /input/package.spk &&
		step "start after the upgrade" control UPGRADE start &&
		check_published_port; then
		libraries=$(api libraries) || libraries=""
		if [[ "$libraries" == *"\"path\":\"/volume1/$share\""* && "$libraries" != */shares/* ]]; then
			record PASS "$label: library listed with the path /volume1/$share"
		else
			record FAIL "$label: library missing or still on the share link: ${libraries:-no answer}"
		fi
		step "stop" stop STOP || :
	fi
	finish_run
}

# run_cpu runs the whole smoke for one CPU model.
run_cpu() {
	local cpu=$1 started=$SECONDS profile ffmpeg_line
	label="$family/$cpu"
	run_dir="$work/${cpu//[^A-Za-z0-9_.-]/_}"
	mkdir -p "$run_dir"
	run_log="$run_dir/dsm.log"
	container="spk-smoke-$family-${cpu//[^A-Za-z0-9_.-]/-}"
	log "=== $label ($(uname_note "$cpu"), syscall list $syscalls) ==="
	start_container "$cpu"
	if install_and_start && check_first_start_state && add_library_through_the_api; then
		for profile in "${PROFILES[@]}"; do smoke_profile "$profile" || :; done
		stop_and_check
		lifecycle || :
	fi
	finish_run
	ffmpeg_line=$(sed -n 's/^\[dsm\] ffmpeg -version: exit 0: //p' "$run_log" | head -n 1)
	record INFO "$label FFmpeg: ${ffmpeg_line:-unknown}; SPK $(wc -c <"$spk") bytes; whole run $((SECONDS - started)) s"
	run_alpha2_upgrade "$cpu"
}

print_summary() {
	local line passed=0 failures=0
	printf '\n[smoke] summary of %s (%s), work directory %s\n' "$family" "$(basename "$spk")" "$work"
	for line in "${results[@]}"; do
		printf '  %s\n' "$line"
		case "$line" in PASS*) passed=$((passed + 1)) ;; FAIL*) failures=$((failures + 1)) ;; esac
	done
	printf 'SUMMARY %s: %d passed, %d failed\n' "$family" "$passed" "$failures"
}

main() {
	parse_args "$@"
	load_family
	prepare
	trap remove_container EXIT
	trap on_signal INT TERM
	if inspect_spk; then
		build_tools
		for cpu in "${cpus[@]}"; do run_cpu "$cpu"; done
	fi
	print_summary
	exit "$failed"
}

main "$@"

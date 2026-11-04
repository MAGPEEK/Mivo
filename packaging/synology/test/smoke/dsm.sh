#!/bin/sh
# dsm.sh - a DSM-like package environment inside the smoke container (task SPK-11). smoke.sh
# calls it through `docker exec` as root, once per step:
#
#   dsm.sh probe                      uname, AT_HWCAP and kernelsim's syscall filter as the
#                                     package user sees them (after install)
#   dsm.sh snapshot <share>           records name, size, time and mode of every file of the share
#   dsm.sh install <spk> <share>      INSTALL: preinst, package.tgz -> target, postinst, then the
#                                     resources DSM acquires before the first start
#   dsm.sh versions                   <id> --version, ffmpeg -version, ffprobe -version
#   dsm.sh control <status> <action>  start-stop-status <action> with SYNOPKG_PKG_STATUS=<status>
#   dsm.sh stop <status>              prestop and stop; fails unless stop exits 0 within 20 s
#   dsm.sh check-stopped <share> [played]
#                                     status 3, no FFmpeg or server process, no transcode
#                                     session left (with "played": the sessions directory must
#                                     exist), share unchanged, a server log without errors
#   dsm.sh upgrade <spk>              UPGRADE in DSM's order, without the final start
#   dsm.sh uninstall <true|false>     UNINSTALL with wizard_delete_data=<true|false>
#   dsm.sh collect                    copies the package and server logs to the state directory
#
# What it models (packaging/synology/DSM-NOTES.md sections 5-8, 10): the package user <id>
# (conf/privilege "run-as": "package"), /var/packages/<id>/{target,var,tmp,home} as links to
# /volume1/@appstore|@appdata|@apptemp|@apphome/<id> (owner <id>, home 0700, the rest 0755),
# the SYNOPKG_* variables of section 5, the execution orders of section 5, script output
# appended to /var/log/packages/<id>.log, the data-share link /var/packages/<id>/shares/<share>
# created after postinst, and target/tmp removed on upgrade and uninstall while var and home
# stay. Every package script and the server run as the package user under kernelsim with the
# syscall list of the family's oldest DSM 7.2 kernel.
#
# Environment (docker run -e, set by smoke.sh): SMOKE_SYSCALLS (kernelsim list), SMOKE_EMULATED
# (yes under QEMU user mode: qemu-shim.sh is appended to the installed common.sh), SMOKE_DSM_ARCH
# (SYNOPKG_DSM_ARCH), SMOKE_FAMILY, SMOKE_KERNEL and SMOKE_X86_CPU (x86_64 only: the version
# commands run through /tools/qemu-x86_64 on that CPU model with that uname). QEMU_CPU and
# QEMU_UNAME are passed on to every package process, so FFmpeg started by the server keeps them.
# Mounts: /tools (kernelsim, sysprobe, qemu-x86_64), /verify (syscall lists), /smoke (this
# directory). POSIX sh; nothing here names the product: the package name comes from INFO.
set -eu

readonly PACKAGES_DIR=/var/packages
readonly VOLUME=/volume1
readonly PACKAGE_LOG_DIR=/var/log/packages
readonly SERVICE_DIR=/usr/local/etc/service.d
readonly STATE_DIR=/var/tmp/smoke-state
# kernelsim's list: the family's list without the names libseccomp does not know.
readonly SYSCALL_LIST=$STATE_DIR/syscalls.txt
readonly SHIM=/smoke/qemu-shim.sh
readonly KERNELSIM=/tools/kernelsim
readonly SYSPROBE=/tools/sysprobe
readonly X86_QEMU=/tools/qemu-x86_64
# A PATH of DSM's shape (system, Synology and local directories). The PATH Package Center
# gives the scripts is not documented; the scripts call only standard tools.
readonly DSM_PATH=/sbin:/bin:/usr/sbin:/usr/bin:/usr/syno/sbin:/usr/syno/bin:/usr/local/sbin:/usr/local/bin
# DSM 7.2.2-72806, the release the 7.2.2 developer guide describes (DSM-NOTES section 3).
readonly DSM_MAJOR=7 DSM_MINOR=2 DSM_BUILD=72806
# Stop budget of the task: start-stop-status waits 20 s before SIGKILL (ADR 0026 section 9).
readonly STOP_BUDGET_SECONDS=20
readonly EXIT_NOT_RUNNING=3
readonly ID_PATTERN='^[a-z0-9][a-z0-9.-]*$'

say() { printf '[dsm] %s\n' "$*"; }
die() {
	printf '[dsm] FAIL %s\n' "$*" >&2
	exit 1
}

# now prints the wall clock in seconds with three decimals (GNU date %N, cut to milliseconds;
# the 10-digit epoch holds until 2286).
now() { date +%s.%N | cut -c1-14; }
# elapsed prints the seconds between $1 and now with two decimals.
elapsed() { awk -v from="$1" -v to="$(now)" 'BEGIN { printf "%.2f", to - from }'; }

# info_value prints the value of key $2 of the INFO file $1 (lines key="value").
info_value() { sed -n "s/^$2=\"\(.*\)\"\$/\1/p" "$1"; }

# load_package sets ID, VERSION and PORT from the INFO file $1.
load_package() {
	[ -f "$1" ] || die "no INFO at $1"
	ID=$(info_value "$1" package)
	VERSION=$(info_value "$1" version)
	PORT=$(info_value "$1" adminport)
	printf '%s\n' "$ID" | grep -Eq "$ID_PATTERN" || die "INFO has no valid package name ('$ID')"
	PKG_ROOT=$PACKAGES_DIR/$ID
	PKG_LOG=$PACKAGE_LOG_DIR/$ID.log
}

# load_installed loads the package that install put in place.
load_installed() {
	set -- "$PACKAGES_DIR"/*/INFO
	[ "$#" -eq 1 ] && [ -f "$1" ] || die "expected exactly one installed package in $PACKAGES_DIR"
	load_package "$1"
}

# unpack_spk extracts the outer SPK archive $1 into the fresh directory $2, as Package Center
# does before preinst (SYNOPKG_PKGINST_TEMP_DIR). The package user must read the scripts there.
unpack_spk() {
	rm -rf "$2"
	mkdir -p "$2"
	tar -xf "$1" -C "$2" --no-same-owner
	[ -f "$2/INFO" ] && [ -f "$2/package.tgz" ] && [ -d "$2/scripts" ] || die "$1 has no INFO, package.tgz or scripts/"
	chmod -R a+rX "$2"
}

ensure_package_user() {
	if ! getent passwd "$ID" >/dev/null; then
		useradd --system --user-group --no-create-home --home-dir "$PKG_ROOT/home" \
			--shell /usr/sbin/nologin "$ID"
	fi
}

# package_dir <kind> <volume dir> <mode>: the real directory on the volume, linked from
# /var/packages/<id>/<kind>, owned by the package user. An existing directory keeps its content.
package_dir() {
	real_dir=$VOLUME/$2/$ID
	mkdir -p "$real_dir"
	chown "$ID:$ID" "$real_dir"
	chmod "$3" "$real_dir"
	ln -sfn "$real_dir" "$PKG_ROOT/$1"
}

create_package_dirs() {
	mkdir -p "$PKG_ROOT" "$PACKAGE_LOG_DIR" "$STATE_DIR"
	package_dir target @appstore 0755
	package_dir var @appdata 0755
	package_dir tmp @apptemp 0755
	package_dir home @apphome 0700
}

# remove_replaced_dirs removes target and tmp, which DSM deletes on upgrade and uninstall;
# var and home are never removed (DSM-NOTES section 8).
remove_replaced_dirs() {
	rm -rf "${VOLUME:?}/@appstore/$ID" "${VOLUME:?}/@apptemp/$ID" "$PKG_ROOT/target" "$PKG_ROOT/tmp"
}

# install_files puts package.tgz of the unpacked SPK $1 into target and its metadata into
# /var/packages/<id>; with run-as package DSM hands the installed files to the package user.
install_files() {
	xz -dc "$1/package.tgz" | tar -x -C "$PKG_ROOT/target/" --no-same-owner
	chown -hR "$ID:$ID" "$VOLUME/@appstore/$ID"
	rm -rf "$PKG_ROOT/scripts" "$PKG_ROOT/conf" "$PKG_ROOT/WIZARD_UIFILES"
	cp -R "$1/scripts" "$1/conf" "$PKG_ROOT/"
	if [ -d "$1/WIZARD_UIFILES" ]; then cp -R "$1/WIZARD_UIFILES" "$PKG_ROOT/"; fi
	cp "$1/INFO" "$PKG_ROOT/INFO"
	chmod -R a+rX "$PKG_ROOT/scripts" "$PKG_ROOT/conf"
	if [ "${SMOKE_EMULATED:-no}" = yes ]; then
		cat "$SHIM" >>"$PKG_ROOT/scripts/common.sh"
		say "QEMU user mode: qemu-shim.sh appended to the installed scripts/common.sh (README.md)"
	fi
}

# as_package_user runs a command as the package user under kernelsim with a clean
# environment: PATH, LANG, QEMU's settings and the NAME=value arguments that precede the
# command (env takes assignments up to the first other argument). kernelsim comes first, so
# env, the command and all its children run under the syscall filter.
as_package_user() {
	prepare_syscall_list
	set -- PATH="$DSM_PATH" LANG=C "$@"
	if [ -n "${QEMU_CPU:-}" ]; then set -- QEMU_CPU="$QEMU_CPU" "$@"; fi
	if [ -n "${QEMU_UNAME:-}" ]; then set -- QEMU_UNAME="$QEMU_UNAME" "$@"; fi
	(cd / && setpriv --reuid="$ID" --regid="$ID" --clear-groups -- \
		"$KERNELSIM" "$SYSCALL_LIST" /usr/bin/env -i "$@")
}

# prepare_syscall_list writes the family's syscall list without the names kernelsim's
# libseccomp does not know (it skips them with one stderr line per name and run, which would
# fill the package log). Those syscalls are newer than libseccomp's table and stay unfiltered;
# probe reports them.
prepare_syscall_list() {
	[ ! -f "$SYSCALL_LIST" ] || return 0
	mkdir -p "$STATE_DIR"
	"$KERNELSIM" "$SMOKE_SYSCALLS" /bin/true 2>"$STATE_DIR/syscalls.unknown" || die "kernelsim does not run"
	sed -n "s/^kernelsim: libseccomp does not know syscall '\([a-z0-9_]*\)', skipped\$/\1/p" \
		"$STATE_DIR/syscalls.unknown" >"$STATE_DIR/syscalls.skipped"
	tr -s '[:space:]' '\n' <"$SMOKE_SYSCALLS" | grep -v '^$' | grep -vxF -f "$STATE_DIR/syscalls.skipped" >"$SYSCALL_LIST.new" || :
	chmod 0644 "$SYSCALL_LIST.new"
	mv "$SYSCALL_LIST.new" "$SYSCALL_LIST"
}

# dsm_variables prints the SYNOPKG_* assignments for status $1 and log file $2 (section 5).
dsm_variables() {
	printf '%s\n' \
		"SYNOPKG_PKGNAME=$ID" "SYNOPKG_PKGVER=$VERSION" "SYNOPKG_PKGDEST=$PKG_ROOT/target" \
		"SYNOPKG_PKGDEST_VOL=$VOLUME" "SYNOPKG_PKGPORT=$PORT" "SYNOPKG_PKGVAR=$PKG_ROOT/var" \
		"SYNOPKG_PKGTMP=$PKG_ROOT/tmp" "SYNOPKG_PKGHOME=$PKG_ROOT/home" "SYNOPKG_PKG_STATUS=$1" \
		"SYNOPKG_OLD_PKGVER=${OLD_VERSION:-}" "SYNOPKG_TEMP_LOGFILE=$2" "SYNOPKG_DSM_LANGUAGE=enu" \
		"SYNOPKG_DSM_VERSION_MAJOR=$DSM_MAJOR" "SYNOPKG_DSM_VERSION_MINOR=$DSM_MINOR" \
		"SYNOPKG_DSM_VERSION_BUILD=$DSM_BUILD" "SYNOPKG_DSM_ARCH=${SMOKE_DSM_ARCH:-}" \
		"SYNOPKG_USERNAME=admin" "SYNOPKG_PKGINST_TEMP_DIR=${INSTALL_TEMP_DIR:-}"
}

# run_script <status> <script path> [wizard assignment] [action] runs one package script the
# way Package Center does and appends its output to /var/log/packages/<id>.log. It prints the
# output, the dialog text of SYNOPKG_TEMP_LOGFILE and the duration, and returns the exit code.
run_script() {
	script_status=$1 script_path=$2 wizard_assignment=${3:-} script_action=${4:-}
	dialog_file=$(mktemp /var/tmp/synopkg-temp-log.XXXXXX)
	output_file=$(mktemp /var/tmp/synopkg-output.XXXXXX)
	chown "$ID:$ID" "$dialog_file"
	# One assignment per line; none of the values contains a newline.
	set --
	while IFS= read -r assignment; do set -- "$@" "$assignment"; done <<EOF
$(dsm_variables "$script_status" "$dialog_file")
EOF
	if [ -n "$wizard_assignment" ]; then set -- "$@" "$wizard_assignment"; fi
	set -- "$@" "$script_path"
	if [ -n "$script_action" ]; then set -- "$@" "$script_action"; fi
	script_started=$(now)
	script_exit=0
	as_package_user "$@" >"$output_file" 2>&1 || script_exit=$?
	script_seconds=$(elapsed "$script_started")
	cat "$output_file" >>"$PKG_LOG"
	sed 's/^/  | /' "$output_file"
	if [ -s "$dialog_file" ]; then sed 's/^/  dialog: /' "$dialog_file"; fi
	rm -f "$dialog_file" "$output_file"
	say "${script_path##*/} ${script_action:+$script_action }(status '${script_status}') exit $script_exit in ${script_seconds} s"
	return "$script_exit"
}

# acquire_resources does what DSM's resource workers do between postinst and the first start
# (DSM-NOTES section 7): port-config copies the .sc file, data-share links the share.
acquire_resources() {
	protocol_file=$(sed -n 's/.*"protocol-file": *"\([^"]*\)".*/\1/p' "$PKG_ROOT/conf/resource")
	[ -n "$protocol_file" ] && [ -f "$PKG_ROOT/target/$protocol_file" ] || die "port-config: no protocol-file in target"
	mkdir -p "$SERVICE_DIR"
	cp "$PKG_ROOT/target/$protocol_file" "$SERVICE_DIR/"
	mkdir -p "$PKG_ROOT/shares"
	ln -sfn "$VOLUME/$1" "$PKG_ROOT/shares/$1"
	say "resources: $SERVICE_DIR/${protocol_file##*/}, $PKG_ROOT/shares/$1 -> $VOLUME/$1 (read-only mount)"
	# The data-share grant gives the package user read access; the read-only mount must allow it.
	as_package_user /bin/ls "$PKG_ROOT/shares/$1/" >/dev/null || die "the package user cannot read the share $1"
}

# config_hash prints the SHA-256 of config.yaml, or "none".
config_hash() {
	if [ -f "$PKG_ROOT/var/config.yaml" ]; then sha256sum <"$PKG_ROOT/var/config.yaml" | cut -d' ' -f1; else echo none; fi
}

cmd_install() {
	[ "$#" -eq 2 ] || die "usage: install <spk> <share>"
	INSTALL_TEMP_DIR=$STATE_DIR/install
	mkdir -p "$STATE_DIR"
	unpack_spk "$1" "$INSTALL_TEMP_DIR"
	load_package "$INSTALL_TEMP_DIR/INFO"
	say "INSTALL $ID $VERSION (adminport $PORT) from $1"
	ensure_package_user
	create_package_dirs
	run_script INSTALL "$INSTALL_TEMP_DIR/scripts/preinst" "wizard_media_share=$2" || die "preinst failed"
	install_files "$INSTALL_TEMP_DIR"
	run_script INSTALL "$PKG_ROOT/scripts/postinst" "wizard_media_share=$2" || die "postinst failed"
	acquire_resources "$2"
	say "config.yaml:"
	sed 's/^/  > /' "$PKG_ROOT/var/config.yaml"
	if [ -f "$STATE_DIR/config.sha256" ]; then
		[ "$(config_hash)" = "$(cat "$STATE_DIR/config.sha256")" ] || die "the reinstall changed the kept config.yaml"
		say "ok the kept config.yaml is unchanged"
	fi
	config_hash >"$STATE_DIR/config.sha256"
}

cmd_probe() {
	load_installed
	say "uname as the package user: $(as_package_user /bin/uname -srm)"
	if [ "${SMOKE_FAMILY:-}" = x86_64 ]; then
		say "uname through qemu-x86_64: $(as_package_user "$X86_QEMU" -cpu "$SMOKE_X86_CPU" -r "$SMOKE_KERNEL" /bin/uname -srm)"
	else
		say "AT_HWCAP: $(as_package_user LD_SHOW_AUXV=1 /bin/true | sed -n 's/^AT_HWCAP: *//p')"
	fi
	say "kernelsim filter: $(as_package_user "$SYSPROBE" | tr '\n' ' ')($(wc -l <"$SYSCALL_LIST") syscalls fail with ENOSYS)"
	if [ -s "$STATE_DIR/syscalls.skipped" ]; then
		say "not filtered (unknown to kernelsim's libseccomp): $(tr '\n' ' ' <"$STATE_DIR/syscalls.skipped")"
	fi
}

# run_version prints the first line of a version command and fails when it fails.
run_version() {
	version_output=$STATE_DIR/version.out
	version_exit=0
	if [ "${SMOKE_FAMILY:-}" = x86_64 ]; then
		as_package_user "$X86_QEMU" -cpu "$SMOKE_X86_CPU" -r "$SMOKE_KERNEL" "$@" >"$version_output" 2>&1 || version_exit=$?
	else
		as_package_user "$@" >"$version_output" 2>&1 || version_exit=$?
	fi
	say "$(printf '%s' "${1##*/} $2" | cut -c1-40): exit $version_exit: $(head -n 1 "$version_output")"
	return "$version_exit"
}

cmd_versions() {
	load_installed
	versions_status=0
	run_version "$PKG_ROOT/target/bin/$ID" --version || versions_status=1
	run_version "$PKG_ROOT/target/bin/ffmpeg" -version || versions_status=1
	run_version "$PKG_ROOT/target/bin/ffprobe" -version || versions_status=1
	return "$versions_status"
}

cmd_control() {
	[ "$#" -eq 2 ] || die "usage: control <status> <action>"
	load_installed
	run_script "$1" "$PKG_ROOT/scripts/start-stop-status" "" "$2"
}

cmd_stop() {
	[ "$#" -eq 1 ] || die "usage: stop <status>"
	load_installed
	run_script "$1" "$PKG_ROOT/scripts/start-stop-status" "" prestop || die "prestop failed"
	stop_started=$(now)
	run_script "$1" "$PKG_ROOT/scripts/start-stop-status" "" stop || die "stop failed"
	stop_seconds=$(elapsed "$stop_started")
	if awk -v seconds="$stop_seconds" -v budget="$STOP_BUDGET_SECONDS" 'BEGIN { exit !(seconds <= budget) }'; then
		say "ok stop finished in $stop_seconds s (budget $STOP_BUDGET_SECONDS s)"
	else
		die "stop took $stop_seconds s, more than $STOP_BUDGET_SECONDS s"
	fi
}

# share_listing prints name, size, modification time and mode of every entry of the share.
share_listing() { find "$VOLUME/$1/" -printf '%P %s %T@ %m\n' | LC_ALL=C sort; }

cmd_snapshot() {
	[ "$#" -eq 1 ] || die "usage: snapshot <share>"
	mkdir -p "$STATE_DIR"
	share_listing "$1" >"$STATE_DIR/share.before"
	say "share $1: $(wc -l <"$STATE_DIR/share.before") entries recorded"
}

# check_share fails when the share differs from the snapshot or is writable.
check_share() {
	share_listing "$1" >"$STATE_DIR/share.after"
	if ! cmp -s "$STATE_DIR/share.before" "$STATE_DIR/share.after"; then
		diff "$STATE_DIR/share.before" "$STATE_DIR/share.after" | head -n 20 >&2 || :
		return 1
	fi
	if touch "$VOLUME/$1/.smoke-write-test" 2>/dev/null; then
		rm -f "$VOLUME/$1/.smoke-write-test"
		say "the share $1 is writable; the read-only mount is missing"
		return 1
	fi
	say "ok share $1 unchanged ($(wc -l <"$STATE_DIR/share.after") entries) and read-only"
}

# package_processes prints PID and command line of every live process that runs the server or
# the bundled FFmpeg, natively (argv0) or under QEMU (argv[1]).
package_processes() {
	for process_dir in /proc/[0-9]*; do
		command_line=$(tr '\000' ' ' <"$process_dir/cmdline" 2>/dev/null) || continue
		case "$command_line" in
		*"$PKG_ROOT/target/bin/"* | *"$VOLUME/@appstore/$ID/bin/"*) printf '%s %s\n' "${process_dir#/proc/}" "$command_line" ;;
		esac
	done
}

# check_sessions fails when the transcode sessions directory holds entries. With $1 = played
# (a playback run with transcode cases came before) the directory must also exist, so an
# absent directory cannot hide output written elsewhere.
check_sessions() {
	sessions_dir=$PKG_ROOT/tmp/transcode/sessions
	if [ ! -d "$sessions_dir" ]; then
		if [ "$1" = played ]; then
			say "FAIL $sessions_dir does not exist after the playback run"
			return 1
		fi
		say "ok $sessions_dir is absent (no playback since the start)"
		return 0
	fi
	sessions_left=$(find "$sessions_dir" -mindepth 1 | wc -l)
	if [ "$sessions_left" -ne 0 ]; then
		say "FAIL $sessions_left entries left in $sessions_dir"
		return 1
	fi
	say "ok $sessions_dir exists and is empty"
}

cmd_check_stopped() {
	[ "$#" -ge 1 ] && [ "$#" -le 2 ] || die "usage: check-stopped <share> [played]"
	case "${2:-}" in '' | played) ;; *) die "check-stopped: second argument must be 'played'" ;; esac
	load_installed
	check_status=0
	status_exit=0
	run_script STOP "$PKG_ROOT/scripts/start-stop-status" "" status || status_exit=$?
	if [ "$status_exit" -eq "$EXIT_NOT_RUNNING" ]; then say "ok status exits 3"; else
		say "FAIL status exits $status_exit, not 3"
		check_status=1
	fi
	left=$(package_processes)
	if [ -z "$left" ]; then say "ok no server or FFmpeg process left"; else
		say "FAIL processes left: $left"
		check_status=1
	fi
	check_sessions "${2:-}" || check_status=1
	check_share "$1" || check_status=1
	server_log=$PKG_ROOT/var/$ID.log
	if [ ! -f "$server_log" ]; then
		say "FAIL $server_log does not exist"
		return 1
	fi
	errors=$(grep -c -e 'level=ERROR' -e '"level":"ERROR"' "$server_log" || :)
	warnings=$(grep -c -e 'level=WARN' -e '"level":"WARN"' "$server_log" || :)
	if [ "${errors:-0}" -eq 0 ]; then say "ok no error in $server_log ($warnings warnings)"; else
		say "FAIL $errors error lines in $server_log:"
		grep -e 'level=ERROR' -e '"level":"ERROR"' "$server_log" | head -n 10
		check_status=1
	fi
	if [ "${warnings:-0}" -gt 0 ]; then grep -e 'level=WARN' -e '"level":"WARN"' "$server_log" | head -n 5 | sed 's/^/  warn: /'; fi
	return "$check_status"
}

cmd_upgrade() {
	[ "$#" -eq 1 ] || die "usage: upgrade <spk>"
	load_installed
	OLD_VERSION=$VERSION
	old_config=$(config_hash)
	INSTALL_TEMP_DIR=$STATE_DIR/upgrade
	unpack_spk "$1" "$INSTALL_TEMP_DIR"
	say "UPGRADE $ID $OLD_VERSION -> $(info_value "$INSTALL_TEMP_DIR/INFO" version)"
	# Old prestop and stop, new preupgrade, old preuninst and postuninst, new preinst, postinst
	# and postupgrade (DSM-NOTES section 5 "Execution order").
	cmd_stop UPGRADE
	load_package "$INSTALL_TEMP_DIR/INFO"
	run_script UPGRADE "$INSTALL_TEMP_DIR/scripts/preupgrade" || die "new preupgrade failed"
	# The old version's scripts see the old SYNOPKG_PKGVER.
	new_version=$VERSION
	VERSION=$OLD_VERSION
	run_script UPGRADE "$PKG_ROOT/scripts/preuninst" || die "old preuninst failed"
	run_script UPGRADE "$PKG_ROOT/scripts/postuninst" || die "old postuninst failed"
	VERSION=$new_version
	remove_replaced_dirs
	create_package_dirs
	run_script UPGRADE "$INSTALL_TEMP_DIR/scripts/preinst" || die "new preinst failed"
	install_files "$INSTALL_TEMP_DIR"
	run_script UPGRADE "$PKG_ROOT/scripts/postinst" || die "new postinst failed"
	run_script UPGRADE "$PKG_ROOT/scripts/postupgrade" || die "new postupgrade failed"
	[ "$(config_hash)" = "$old_config" ] || die "the upgrade changed config.yaml"
	[ -n "$(find "$PKG_ROOT/var/data" -mindepth 1 -type f)" ] || die "the upgrade lost $PKG_ROOT/var/data"
	say "ok config.yaml unchanged, $(find "$PKG_ROOT/var/data" -mindepth 1 -type f | wc -l) files kept in var/data"
}

cmd_uninstall() {
	[ "$#" -eq 1 ] || die "usage: uninstall <true|false>"
	case "$1" in true | false) ;; *) die "uninstall needs true or false" ;; esac
	load_installed
	say "UNINSTALL $ID $VERSION (wizard_delete_data=$1)"
	cmd_stop UNINSTALL
	run_script UNINSTALL "$PKG_ROOT/scripts/preuninst" "wizard_delete_data=$1" || die "preuninst failed"
	remove_replaced_dirs
	run_script UNINSTALL "$PKG_ROOT/scripts/postuninst" "wizard_delete_data=$1" || die "postuninst failed"
	protocol_file=$(sed -n 's/.*"protocol-file": *"\([^"]*\)".*/\1/p' "$PKG_ROOT/conf/resource")
	rm -f "$SERVICE_DIR/${protocol_file##*/}"
	rm -rf "$PKG_ROOT"
	kept=$(find "$VOLUME/@appdata/$ID" "$VOLUME/@apphome/$ID" -mindepth 1 | wc -l)
	if [ "$1" = false ]; then
		[ -f "$VOLUME/@appdata/$ID/config.yaml" ] && [ -n "$(find "$VOLUME/@appdata/$ID/data" -mindepth 1 -type f)" ] ||
			die "var lost config.yaml or data/ although the data was to be kept"
		say "ok var and home kept ($kept entries)"
	else
		[ "$kept" -eq 0 ] || die "$kept entries left in var and home although the data was to be deleted"
		rm -f "$STATE_DIR/config.sha256"
		say "ok var and home emptied, the directories stay"
	fi
}

cmd_collect() {
	load_installed
	mkdir -p "$STATE_DIR/logs"
	cp "$PKG_LOG" "$STATE_DIR/logs/" 2>/dev/null || :
	cp "$PKG_ROOT/var/$ID.log" "$STATE_DIR/logs/server-$(date +%H%M%S).log" 2>/dev/null || :
	chmod -R a+rX "$STATE_DIR/logs"
}

main() {
	[ "$#" -ge 1 ] || die "usage: dsm.sh <command> [arguments]"
	command_name=$1
	shift
	case "$command_name" in
	probe) cmd_probe "$@" ;;
	snapshot) cmd_snapshot "$@" ;;
	install) cmd_install "$@" ;;
	versions) cmd_versions "$@" ;;
	control) cmd_control "$@" ;;
	stop) cmd_stop "$@" ;;
	check-stopped) cmd_check_stopped "$@" ;;
	upgrade) cmd_upgrade "$@" ;;
	uninstall) cmd_uninstall "$@" ;;
	collect) cmd_collect "$@" ;;
	*) die "unknown command $command_name" ;;
	esac
}

main "$@"

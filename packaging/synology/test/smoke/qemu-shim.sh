# shellcheck shell=sh
# shellcheck disable=SC2329 # is_server_process is called by start-stop-status, not here
# qemu-shim.sh - appended by dsm.sh to the *installed* copy of scripts/common.sh when the
# family runs under QEMU user-mode emulation (armv8, armv7; task SPK-11). Never part of an SPK.
#
# Why: start-stop-status recognises its server by `[ /proc/<pid>/exe -ef $PKG_BINARY ]`. Under
# QEMU user mode (binfmt_misc) the kernel's /proc/<pid>/exe of an emulated process names the
# interpreter (/usr/bin/qemu-arm, qemu-aarch64), never the guest binary, and QEMU emulates the
# link only for a process's own PID. Without this shim every emulated start would end with
# "did not start within 5 s" although the server runs - an artefact of the emulator, not of the
# package (README.md, "QEMU and the process check").
#
# How: start-stop-status defines is_server_process before it sources common.sh, so this later
# definition replaces it. The original test runs first and is the only one that can pass on a
# real NAS; the fallback accepts a live process whose exe is a QEMU interpreter and whose guest
# binary (argv[1] of the interpreter with binfmt's preserve-argv0 flag: interpreter, guest
# path, argv0, arguments) is the package binary. Everything else start-stop-status does - PID
# file, health check, SIGTERM, stop budget, SIGKILL - stays the package's own code.
is_server_process() {
	case "$1" in
	'' | *[!0-9]*) return 1 ;;
	esac
	# shellcheck disable=SC3013 # -ef is supported by BusyBox ash, bash and dash
	if [ "/proc/$1/exe" -ef "$PKG_BINARY" ]; then
		return 0
	fi
	smoke_interpreter=$(readlink "/proc/$1/exe" 2>/dev/null) || return 1
	case "${smoke_interpreter##*/}" in
	qemu-*) ;;
	*) return 1 ;;
	esac
	process_alive "$1" || return 1
	smoke_guest=$(tr '\000' '\n' <"/proc/$1/cmdline" 2>/dev/null | sed -n 2p)
	# shellcheck disable=SC3013
	[ -n "$smoke_guest" ] && [ "$smoke_guest" -ef "$PKG_BINARY" ]
}

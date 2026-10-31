#!/usr/bin/env bash
# verify.sh - check one family's static FFmpeg under DSM 7.2-like conditions (task SPK-15).
#
#   verify.sh <family> <binaries-dir> [cpu-model ...]
#
# <binaries-dir> holds ffmpeg and ffprobe of the family (the output of the Dockerfile target).
# Every run uses a debian:bookworm-slim container of the family's platform (glibc 2.36, as the
# DSM 7.2 toolchains), the syscall surface of the family's oldest DSM 7.2 kernel (kernelsim),
# uname reporting that kernel, and a baseline CPU model (defaults below). Inside, checks.sh runs
# the capability, corpus, hostile-input and HLS checks, then the server's own FFmpeg adapter
# tests (go test ./internal/adapters/ffmpeg) against the build.
#
# Needs: bash, Docker on an x86_64 host with binfmt QEMU for arm64 and arm/v7 (Docker Desktop),
# the corpus in testdata/media (go run ./scripts/gen-testmedia). Runs from Git Bash on Windows
# and from Linux shells. Environment: VERIFY_WORK (work directory, default: a new temporary
# directory), VERIFY_GO_TESTS=0 skips the Go tests. Exit status 1 if any check failed.
set -euo pipefail

# Git Bash must not rewrite container paths such as /tools into Windows paths.
export MSYS_NO_PATHCONV=1

readonly BOOKWORM_IMAGE=debian:bookworm-slim@sha256:99c0b25c46d0214d5a2091092e7908907dcaecd6a962cde0ac700acee222b6f3
# Must match the toolchain line of go.mod.
readonly GO_IMAGE=golang:1.27.3@sha256:2920bb124f1a2e12a1d39f0cab756ae0068441ee1da7655dec7f5e374944095b

script_dir=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$script_dir/../../../../.." && pwd)

usage() {
	printf 'usage: %s <x86_64|armv8|armv7> <binaries-dir> [cpu-model ...]\n' "$0" >&2
	exit 2
}

# host_path converts a path for docker -v; Docker Desktop on Windows needs C:/... paths.
host_path() {
	if command -v cygpath >/dev/null 2>&1; then cygpath -m "$1"; else printf '%s' "$1"; fi
}

# load_family sets the platform, Go architecture, oldest kernel and default CPU models.
load_family() {
	case "$1" in
	x86_64) platform=linux/amd64 goarch=amd64 kernel=3.10.108 syscalls=syscalls-after-3.10.txt
		default_cpus=(Opteron_G1) ;;
	armv8) platform=linux/arm64 goarch=arm64 kernel=4.4.302 syscalls=syscalls-after-4.4.txt
		default_cpus=(cortex-a53) ;;
	# cortex-a9 without NEON and with 16 double registers is the Armada 385/388 worst case.
	armv7) platform=linux/arm/v7 goarch=arm kernel=3.10.108 syscalls=syscalls-after-3.10.txt
		default_cpus=(cortex-a9 cortex-a15 "cortex-a9,neon=off,vfp-d32=off") ;;
	*) usage ;;
	esac
}

# server_whitelist prints allowedDemuxers of probe.go, the server's -format_whitelist without
# image2 (checks.sh adds image2 like inputDemuxers does).
server_whitelist() {
	sed -n '/^func allowedDemuxers/,/^}/p' "$repo/internal/adapters/ffmpeg/probe.go" |
		grep -o '"[a-z0-9_]*"' | tr -d '"' | paste -sd, -
}

build_tools() {
	docker build --quiet --output "type=local,dest=$(host_path "$work/tools")" "$(host_path "$script_dir")" >/dev/null
}

# build_go_tests cross-compiles the FFmpeg adapter's test binary for the family.
build_go_tests() {
	mkdir -p "$work/gotest"
	docker run --rm -v "$(host_path "$repo"):/src:ro" -v "$(host_path "$work/gotest"):/out" \
		-v ffmpeg-verify-gomod:/go/pkg/mod -v ffmpeg-verify-gocache:/root/.cache/go-build -w /src \
		-e CGO_ENABLED=0 -e GOOS=linux -e GOARCH="$goarch" -e GOARM=7 -e GOFLAGS=-buildvcs=false \
		"$GO_IMAGE" go test -c -o /out/ffmpeg.test ./internal/adapters/ffmpeg
}

# run_checks runs checks.sh for one CPU model and keeps its output in the work directory.
run_checks() {
	cpu=$1
	log="$work/$family-${cpu//[^A-Za-z0-9_.-]/_}.log"
	emulation=()
	if [ "$family" != x86_64 ]; then emulation=(-e "QEMU_CPU=$cpu" -e "QEMU_UNAME=$kernel"); fi
	printf '== %s on %s (kernel %s surface)\n' "$family" "$cpu" "$kernel"
	docker run --rm --platform "$platform" \
		-v "$(host_path "$work/tools"):/tools:ro" -v "$(host_path "$script_dir"):/verify:ro" \
		-v "$(host_path "$binaries"):/ff:ro" -v "$(host_path "$repo/testdata/media"):/corpus:ro" \
		-v "$(host_path "$repo"):/repo:ro" -v "$(host_path "$work/gotest"):/gotest:ro" \
		-e "FAMILY=$family" -e "CPU_MODEL=$cpu" -e "DSM_KERNEL=$kernel" -e "WHITELIST=$whitelist" \
		"${emulation[@]}" "$BOOKWORM_IMAGE" \
		/tools/kernelsim "/verify/$syscalls" /bin/sh /verify/checks.sh 2>&1 | tee "$log"
}

main() {
	[ "$#" -ge 2 ] || usage
	family=$1
	binaries=$(cd "$2" && pwd)
	shift 2
	load_family "$family"
	cpus=("$@")
	[ "${#cpus[@]}" -gt 0 ] || cpus=("${default_cpus[@]}")
	[ -f "$repo/testdata/media/manifest.json" ] || {
		printf 'verify.sh: testdata/media is missing; run go run ./scripts/gen-testmedia\n' >&2
		exit 2
	}
	work=${VERIFY_WORK:-$(mktemp -d)}
	mkdir -p "$work"
	whitelist=$(server_whitelist)
	printf 'verify.sh: work directory %s, server whitelist %s\n' "$work" "$whitelist"
	build_tools
	if [ "${VERIFY_GO_TESTS:-1}" != 0 ]; then build_go_tests; else mkdir -p "$work/gotest"; fi
	status=0
	for cpu in "${cpus[@]}"; do run_checks "$cpu" || status=1; done
	grep -h '^SUMMARY' "$work/$family"-*.log
	exit "$status"
}

main "$@"

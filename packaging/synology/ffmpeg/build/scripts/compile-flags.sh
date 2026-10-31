# compile-flags.sh - flags shared by the libraries and FFmpeg (sourced after layout.sh, POSIX sh).
#
# HARDENING_CFLAGS: stack protector for C code that parses untrusted media files (musl provides
#   __stack_chk_fail); -ffile-prefix-map keeps the build directory out of assertion strings.
# STATIC_LDFLAGS: fully static executables. musl's default thread stack is 128 KiB; FFmpeg,
#   x264 and dav1d run deep call chains on worker threads, so the PT_GNU_STACK size of the
#   executable raises musl's default thread stack to 2 MiB (musl reads it at start-up).

# shellcheck shell=sh

readonly THREAD_STACK_BYTES=2097152
HARDENING_CFLAGS="-fstack-protector-strong -ffile-prefix-map=$BUILD_ROOT=."
STATIC_LDFLAGS="-static -Wl,-z,stack-size=$THREAD_STACK_BYTES"
export HARDENING_CFLAGS STATIC_LDFLAGS

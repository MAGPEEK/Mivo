# families.sh - target definition of each architecture family (sourced, POSIX sh).
#
# The baselines are those of ADR 0026 Notes: x86-64 v1 (no -march above x86-64), armv8-a, and
# armv7-a hard-float with VFPv3-D16. NEON on armv7 is never assumed at compile time; FFmpeg,
# x264 and dav1d select their NEON code at run time (README.md "Baselines").
#
# load_family <family> sets:
#   TRIPLET            musl cross toolchain target
#   GCC_TARGET_CONFIG  gcc configure options that make the baseline the toolchain default, so
#                      musl and libgcc are built for it as well
#   ARCH_CFLAGS        the baseline as compiler flags, passed to every library and FFmpeg
#   FFMPEG_ARCH        FFmpeg's --arch;  FFMPEG_CPU  FFmpeg's --cpu (becomes -march)
#   MESON_CPU_FAMILY, MESON_CPU   host_machine of the meson cross file (dav1d)
#   OLDEST_KERNEL      oldest DSM 7.2 kernel of the family; an ELF ABI note must not exceed it

# shellcheck shell=sh

load_family() {
	case "$1" in
	x86_64)
		TRIPLET=x86_64-linux-musl
		GCC_TARGET_CONFIG="--with-arch=x86-64 --with-tune=generic"
		ARCH_CFLAGS="-march=x86-64 -mtune=generic"
		FFMPEG_ARCH=x86_64
		FFMPEG_CPU=x86-64
		MESON_CPU_FAMILY=x86_64
		MESON_CPU=x86_64
		OLDEST_KERNEL=3.10.108
		;;
	armv8)
		TRIPLET=aarch64-linux-musl
		GCC_TARGET_CONFIG="--with-arch=armv8-a"
		ARCH_CFLAGS="-march=armv8-a"
		FFMPEG_ARCH=aarch64
		FFMPEG_CPU=armv8-a
		MESON_CPU_FAMILY=aarch64
		MESON_CPU=armv8-a
		OLDEST_KERNEL=4.4.302
		;;
	armv7)
		TRIPLET=arm-linux-musleabihf
		GCC_TARGET_CONFIG="--with-arch=armv7-a --with-fpu=vfpv3-d16 --with-float=hard --with-mode=arm"
		ARCH_CFLAGS="-march=armv7-a -mfpu=vfpv3-d16 -mfloat-abi=hard -marm"
		FFMPEG_ARCH=arm
		FFMPEG_CPU=armv7-a
		MESON_CPU_FAMILY=arm
		MESON_CPU=armv7
		OLDEST_KERNEL=3.10.108
		;;
	*)
		printf 'unknown family: %s (expected x86_64, armv8 or armv7)\n' "$1" >&2
		return 2
		;;
	esac
	FAMILY=$1
	export FAMILY TRIPLET GCC_TARGET_CONFIG ARCH_CFLAGS FFMPEG_ARCH FFMPEG_CPU MESON_CPU_FAMILY MESON_CPU OLDEST_KERNEL
}

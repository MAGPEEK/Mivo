#!/bin/sh
# checks.sh - capability, corpus, hostile-input and HLS checks of one FFmpeg build, run by
# verify.sh inside a debian:bookworm-slim container of the family's platform, under kernelsim.
#
# Environment (set by verify.sh):
#   FAMILY      x86_64, armv8 or armv7
#   CPU_MODEL   QEMU CPU model the binaries run on
#   DSM_KERNEL  kernel release uname reports
#   WHITELIST   the server's -format_whitelist without image2 (from probe.go)
# Mounts: /ff (ffmpeg, ffprobe), /corpus (testdata/media), /tools (verify tools),
#         /repo (repository, read-only) and /gotest (Go test binary, optional).
#
# Prints one "PASS <check>" or "FAIL <check>" line per check and "SUMMARY <passed>/<total>";
# exits 1 if any check failed.
set -u
: "${FAMILY:?}" "${CPU_MODEL:?}" "${DSM_KERNEL:?}" "${WHITELIST:?}"

failures=0
total=0
bin=/tmp/bin
scratch=/tmp/checks
mkdir -p "$bin" "$scratch"

pass() {
	total=$((total + 1))
	printf 'PASS %s\n' "$*"
}

failed() {
	total=$((total + 1))
	failures=$((failures + 1))
	printf 'FAIL %s\n' "$*"
}

# check <description> <command...>: PASS when the command succeeds.
check() {
	description=$1
	shift
	if "$@"; then pass "$description"; else failed "$description"; fi
}

# install_wrappers puts ffmpeg and ffprobe on PATH. x86_64 binaries run natively on the
# build machine, so they go through qemu-x86_64 with the baseline CPU; arm binaries already
# run under the binfmt QEMU, which reads QEMU_CPU and QEMU_UNAME from the environment.
install_wrappers() {
	for tool in ffmpeg ffprobe; do
		if [ "$FAMILY" = x86_64 ]; then
			printf '#!/bin/sh\nexec /tools/qemu-x86_64 -cpu %s -r %s /ff/%s "$@"\n' "$CPU_MODEL" "$DSM_KERNEL" "$tool" >"$bin/$tool"
		else
			printf '#!/bin/sh\nexec /ff/%s "$@"\n' "$tool" >"$bin/$tool"
		fi
		chmod 0755 "$bin/$tool"
	done
	PATH="$bin:$PATH"
	export PATH
}

report_environment() {
	if [ "$FAMILY" = x86_64 ]; then
		emulated_uname=$(/tools/qemu-x86_64 -cpu "$CPU_MODEL" -r "$DSM_KERNEL" /bin/uname -rm)
	else
		emulated_uname=$(uname -rm)
		printf 'INFO hwcap: %s\n' "$(LD_SHOW_AUXV=1 /bin/true | sed -n 's/^AT_HWCAP: *//p')"
	fi
	printf 'INFO family %s, cpu %s, uname %s, glibc %s\n' "$FAMILY" "$CPU_MODEL" "$emulated_uname" \
		"$(ldd --version 2>&1 | sed -n '1s/.* //p')"
	printf 'INFO syscall filter: %s\n' "$(/tools/sysprobe | tr '\n' ' ')"
	case "$emulated_uname" in "$DSM_KERNEL "*) pass "uname reports $DSM_KERNEL" ;; *) failed "uname reports $DSM_KERNEL" ;; esac
}

check_version() {
	ffmpeg -hide_banner -version >"$scratch/version" 2>&1 || {
		failed "ffmpeg -version"
		return
	}
	printf 'INFO %s\n' "$(head -n 1 "$scratch/version")"
	printf 'INFO %s\n' "$(grep '^configuration:' "$scratch/version")"
	configuration=$(grep '^configuration:' "$scratch/version")
	case "$configuration" in
	*--enable-gpl*--enable-version3*) pass "configuration has --enable-gpl --enable-version3" ;;
	*) failed "configuration has --enable-gpl --enable-version3" ;;
	esac
	case "$configuration" in
	*nonfree* | *fdk*) failed "configuration has no nonfree and no libfdk-aac" ;;
	*) pass "configuration has no nonfree and no libfdk-aac" ;;
	esac
	check "ffprobe -version" ffprobe -hide_banner -version >/dev/null
	ffmpeg -hide_banner -L >"$scratch/licence" 2>&1
	check "ffmpeg -L: GPL version 3 or later" grep -q 'GNU General Public License as published by' "$scratch/licence"
	check "ffmpeg -L: version 3" grep -q 'either version 3 of the License' "$scratch/licence"
}

# listed <listing> <name>: the name appears in the name column (aliases split at commas).
listed() {
	awk -v wanted="$2" 'seen { n = split($2, names, ","); for (i = 1; i <= n; i++) if (names[i] == wanted) found = 1 }
		/^ *-+ *$/ { seen = 1 } END { exit found ? 0 : 1 }' "$1"
}

check_components() {
	ffmpeg -hide_banner -encoders >"$scratch/encoders" 2>/dev/null
	ffmpeg -hide_banner -muxers >"$scratch/muxers" 2>/dev/null
	ffmpeg -hide_banner -demuxers >"$scratch/demuxers" 2>/dev/null
	for encoder in libx264 aac; do check "encoder $encoder" listed "$scratch/encoders" "$encoder"; done
	for muxer in hls mp4; do check "muxer $muxer" listed "$scratch/muxers" "$muxer"; done
	missing=""
	for demuxer in $(printf '%s' "$WHITELIST,image2" | tr ',' ' '); do
		listed "$scratch/demuxers" "$demuxer" || missing="$missing $demuxer"
	done
	if [ -z "$missing" ]; then pass "every whitelisted demuxer is built ($WHITELIST,image2)"; else failed "demuxers missing:$missing"; fi
	for absent in concat hls dash imf vobsub; do
		if listed "$scratch/demuxers" "$absent"; then failed "demuxer $absent is not built"; else pass "demuxer $absent is not built"; fi
	done
	printf 'INFO input protocols: %s\n' "$(ffmpeg -hide_banner -protocols 2>/dev/null | awk '/^Input:/{i=1;next}/^Output:/{i=0}i{printf "%s ", $1}')"
	for tool in ffmpeg ffprobe; do
		"$tool" -hide_banner -h full >"$scratch/help-$tool" 2>/dev/null
		check "$tool -h full lists format_whitelist" grep -q 'format_whitelist' "$scratch/help-$tool"
	done
}

# whitelist_for prints the server's whitelist for a path: image2 only without "%" (probe.go).
whitelist_for() {
	case "$1" in *%*) printf '%s' "$WHITELIST" ;; *) printf '%s,image2' "$WHITELIST" ;; esac
}

# expected_formats maps every corpus file (and sidecar) of the manifest to its probeFormat.
expected_formats() {
	awk -F'"' '/"file":/ { file = $4 } /"probeFormat":/ { print file, $4 }' /corpus/manifest.json
}

check_corpus() {
	expected_formats >"$scratch/expected"
	count=0
	probed=0
	matched=0
	for path in $(cd /corpus && find . -type f ! -name manifest.json | sed 's|^\./||' | sort); do
		count=$((count + 1))
		if ffprobe -v error -hide_banner -print_format json -show_format -show_streams \
			-format_whitelist "$(whitelist_for "/corpus/$path")" -protocol_whitelist file \
			"file:/corpus/$path" >"$scratch/probe.json" 2>"$scratch/probe.err"; then
			probed=$((probed + 1))
		else
			printf 'INFO probe failed: %s: %s\n' "$path" "$(tr '\n' ' ' <"$scratch/probe.err")"
		fi
		format=$(sed -n 's/^ *"format_name": "\(.*\)",$/\1/p' "$scratch/probe.json")
		want=$(awk -v file="$path" '$1 == file { print $2 }' "$scratch/expected")
		if [ -n "$want" ] && [ "$format" = "$want" ]; then
			matched=$((matched + 1))
		else
			printf 'INFO format of %s: %s (manifest: %s)\n' "$path" "$format" "$want"
		fi
	done
	if [ "$probed" -eq "$count" ] && [ "$count" -gt 0 ]; then
		pass "corpus probed with the server's arguments: $probed/$count"
	else
		failed "corpus probed with the server's arguments: $probed/$count"
	fi
	if [ "$matched" -eq "$count" ]; then pass "corpus format names match the manifest: $matched/$count"; else failed "corpus format names match the manifest: $matched/$count"; fi
}

# read_through_descriptor <whitelist> <path>: open the file like the Linux producer does, as
# the inherited descriptor 3 (/proc/self/fd/3, no file name extension).
read_through_descriptor() {
	ffmpeg -hide_banner -nostdin -v error -format_whitelist "$1" -protocol_whitelist file \
		-i file:/proc/self/fd/3 -f null - 3<"$2"
}

# refused <command...>: the command fails and printed no h264 stream of the canary.
refused() {
	if "$@" >"$scratch/refused.out" 2>&1; then return 1; fi
	! grep -q h264 "$scratch/refused.out"
}

check_hostile() {
	library=/tmp/library
	mkdir -p "$library"
	cp /corpus/video/h264_aac.mkv "$library/canary.mkv"
	printf 'ffconcat version 1.0\nfile canary.mkv\n' >"$library/evil.mkv"
	printf '#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:2\n#EXTINF:2.0,\ncanary.mkv\n#EXT-X-ENDLIST\n' >"$library/evil.m3u8"
	if ffprobe -v quiet -protocol_whitelist file -show_entries stream=codec_name -print_format csv \
		"file:$library/evil.mkv" 2>/dev/null | grep -q h264; then
		printf 'INFO an unrestricted ffprobe follows the ffconcat list\n'
	else
		printf 'INFO an unrestricted ffprobe does not follow the ffconcat list (no concat demuxer)\n'
	fi
	for name in evil.mkv evil.m3u8; do
		check "hostile $name refused by the probe" refused ffprobe -v error -hide_banner -show_streams \
			-format_whitelist "$(whitelist_for "$library/$name")" -protocol_whitelist file "file:$library/$name"
		check "hostile $name refused through /proc/self/fd/3" refused \
			read_through_descriptor "$(whitelist_for "$library/$name")" "$library/$name"
	done
	ffprobe -v error -format_whitelist matroska -protocol_whitelist file file:/corpus/audio/pcm_s16le.wav >"$scratch/enforced" 2>&1
	check "-format_whitelist refuses a demuxer not on it" grep -q 'not on whitelist' "$scratch/enforced"
}

# transcode_hls <source> <out>: 3 s of the source with the producer's options (hlsargs.go),
# read through the inherited descriptor like the Linux producer.
transcode_hls() {
	whitelist=$(whitelist_for "$1")
	ffmpeg -hide_banner -nostdin -loglevel error -y \
		-format_whitelist "$whitelist" -protocol_whitelist file -i file:/proc/self/fd/3 \
		-map 0:0 -map 0:1 -sn -dn -map_metadata -1 -map_chapters -1 \
		-c:v libx264 -preset veryfast -crf 23 -profile:v high -pix_fmt yuv420p \
		-force_key_frames 'expr:gte(t,n_forced*6)' -sc_threshold 0 -vf scale=-2:480 \
		-c:a aac -ac 2 -b:a 192k -t 3 \
		-f hls -hls_time 6 -hls_playlist_type event -hls_segment_type fmp4 -hls_fmp4_init_filename init.mp4 \
		-hls_flags independent_segments+temp_file -start_number 0 \
		-hls_segment_filename "file:$2/seg-%05d.m4s" "file:$2/index.m3u8" 3<"$1"
}

hls_video_ok() { grep -qx 'codec_name=h264' "$1" && grep -qx 'profile=High' "$1" && grep -qx 'pix_fmt=yuv420p' "$1"; }

hls_audio_ok() { grep -qx 'codec_name=aac' "$1" && grep -qx 'channels=2' "$1"; }

# check_hls transcodes HEVC Main 10 + AC-3 to fMP4 HLS and checks the output.
check_hls() {
	out=/tmp/hls
	mkdir -p "$out"
	started=$(date +%s)
	if ! transcode_hls /corpus/video/hevc_main10_ac3.mkv "$out" 2>"$scratch/hls.err"; then
		failed "HLS transcode: $(tr '\n' ' ' <"$scratch/hls.err")"
		return
	fi
	printf 'INFO HLS transcode took %ss under emulation\n' "$(($(date +%s) - started))"
	check "HLS playlist maps init.mp4" grep -q '#EXT-X-MAP:URI="init.mp4"' "$out/index.m3u8"
	check "HLS playlist is complete" grep -q '#EXT-X-ENDLIST' "$out/index.m3u8"
	cat "$out/init.mp4" "$out"/seg-*.m4s >"$scratch/joined.mp4"
	ffprobe -v error -show_entries stream=codec_name,profile,pix_fmt,channels:format=duration \
		-of default=noprint_wrappers=1 "file:$scratch/joined.mp4" >"$scratch/hls.probe" 2>&1
	printf 'INFO HLS output: %s\n' "$(tr '\n' ' ' <"$scratch/hls.probe")"
	check "HLS video is h264 High yuv420p" hls_video_ok "$scratch/hls.probe"
	check "HLS audio is aac stereo" hls_audio_ok "$scratch/hls.probe"
	duration=$(sed -n 's/^duration=\([0-9]*\).*/\1/p' "$scratch/hls.probe")
	check "HLS output covers the 2 s clip within -t 3 (got ${duration:-?} s)" test "${duration:-0}" -ge 2 -a "${duration:-0}" -le 3
}

# run_go_tests runs the server's own FFmpeg adapter tests (corpus probe against the manifest,
# hostile inputs, keyframes, HLS producer) against this build.
run_go_tests() {
	[ -x /gotest/ffmpeg.test ] || return 0
	cd /repo/internal/adapters/ffmpeg || return 0
	started=$(date +%s)
	/gotest/ffmpeg.test -test.v -test.timeout 90m >"$scratch/gotest.log" 2>&1
	status=$?
	printf 'INFO Go adapter tests took %ss: %s passed, %s skipped, %s failed (top-level tests)\n' \
		"$(($(date +%s) - started))" "$(grep -c '^--- PASS' "$scratch/gotest.log")" \
		"$(grep -c '^--- SKIP' "$scratch/gotest.log")" "$(grep -c '^--- FAIL' "$scratch/gotest.log")"
	grep -A 3 -E '^--- SKIP' "$scratch/gotest.log" | grep -E '^--- SKIP|skipping' | sed 's/^/INFO /'
	if [ "$status" -eq 0 ]; then
		pass "server's FFmpeg adapter tests (go test ./internal/adapters/ffmpeg)"
	else
		grep -E -B 2 -A 12 -- '--- FAIL' "$scratch/gotest.log" | head -n 150
		failed "server's FFmpeg adapter tests (go test ./internal/adapters/ffmpeg)"
	fi
}

install_wrappers
report_environment
check_version
check_components
check_corpus
check_hostile
check_hls
run_go_tests
printf 'SUMMARY %s %s: %s/%s checks passed\n' "$FAMILY" "$CPU_MODEL" "$((total - failures))" "$total"
[ "$failures" -eq 0 ]

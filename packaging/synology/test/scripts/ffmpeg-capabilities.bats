#!/usr/bin/env bats
# Tests of ffmpeg-capabilities.sh (task M3-16j) with the listing fake of ../fake-ffmpeg.sh: the
# disc demuxers and protocols and the photo decoders are required, HEIC/AVIF decoders are only
# reported. Each case runs once per shell of SCRIPT_SHELLS (helpers.bash); how to run them:
# ../README.md.

bats_require_minimum_version 1.11.0

load helpers

setup() {
	# A copy with the executable bit, whatever mode the checkout gave the file.
	FAKE_FFMPEG=$BATS_TEST_TMPDIR/ffmpeg
	cp "$TEST_DIR/fake-ffmpeg.sh" "$FAKE_FFMPEG"
	chmod 0755 "$FAKE_FFMPEG"
	unset FAKE_FFMPEG_MISSING FAKE_FFMPEG_SILENT
}

# run_capabilities runs the script under the shell of the current case with the given arguments.
run_capabilities() {
	run_script "$PACKAGE_SCRIPTS/ffmpeg-capabilities.sh" "$@"
}

case_complete_build_passes() {
	run_capabilities "$FAKE_FFMPEG"
	assert_success
	assert_line --index 0 "ffmpeg capabilities of $FAKE_FFMPEG: ffmpeg version 9.9-fake Copyright (c) 2000-2026 the FFmpeg developers"
	local name
	for name in "demuxer concat" "demuxer mpeg" "demuxer mpegts" "protocol file" "protocol concat" \
		"decoder mjpeg" "decoder png" "decoder webp" "decoder gif" "decoder tiff"; do
		assert_line "required $name: ok"
	done
	assert_line "optional hevc decoder: available"
	assert_line "optional heif (mov demuxer and hevc decoder): available"
	assert_line "optional libdav1d decoder (AVIF, AV1): available"
	assert_line "summary: required capabilities present"
	assert_equal "$stderr" ""
}

case_missing_disc_and_photo_capabilities_fail() {
	export FAKE_FFMPEG_MISSING="concat mpegts tiff"
	run_capabilities "$FAKE_FFMPEG"
	assert_failure 1
	assert_line "required demuxer concat: MISSING"
	assert_line "required demuxer mpegts: MISSING"
	# The fake still lists concat under Output:; only the Input: section counts.
	assert_line "required protocol concat: MISSING"
	assert_line "required decoder tiff: MISSING"
	assert_line "required demuxer mpeg: ok"
	assert_line "summary: required capabilities missing: demuxer:concat demuxer:mpegts protocol:concat decoder:tiff"
}

case_missing_optional_decoders_only_report() {
	export FAKE_FFMPEG_MISSING="hevc libdav1d"
	run_capabilities "$FAKE_FFMPEG"
	assert_success
	assert_line "optional hevc decoder: not available"
	assert_line "optional heif (mov demuxer and hevc decoder): not available"
	assert_line "optional libdav1d decoder (AVIF, AV1): not available"
	assert_line "summary: required capabilities present"
	export FAKE_FFMPEG_MISSING="mov"
	run_capabilities "$FAKE_FFMPEG"
	assert_success
	assert_line "optional hevc decoder: available"
	assert_line "optional heif (mov demuxer and hevc decoder): not available"
}

case_device_rows_and_aliases_are_read_by_name() {
	# "alsa" is a device row (" D d alsa"); its "d" flag is no demuxer named d.
	export FAKE_FFMPEG_MISSING="concat"
	run_capabilities "$FAKE_FFMPEG"
	assert_failure 1
	refute_line --partial "demuxer d:"
	# mov is found among the aliases "mov,mp4,m4a,...".
	assert_line "optional heif (mov demuxer and hevc decoder): available"
}

case_a_binary_that_lists_nothing_is_a_usage_error() {
	export FAKE_FFMPEG_SILENT=1
	run_capabilities "$FAKE_FFMPEG"
	assert_failure 2
	assert_output ''
	assert_stderr_contains "cannot list its demuxers, decoders and protocols"
	unset FAKE_FFMPEG_SILENT
	run_capabilities "$BATS_TEST_TMPDIR/no-such-ffmpeg"
	assert_failure 2
}

case_wrong_argument_count_is_a_usage_error() {
	run_capabilities
	assert_failure 2
	assert_stderr_contains "usage:"
	run_capabilities "$FAKE_FFMPEG" extra
	assert_failure 2
}

register_cases \
	complete_build_passes \
	missing_disc_and_photo_capabilities_fail \
	missing_optional_decoders_only_report \
	device_rows_and_aliases_are_read_by_name \
	a_binary_that_lists_nothing_is_a_usage_error \
	wrong_argument_count_is_a_usage_error

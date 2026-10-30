package main

import "slices"

// ffprobe reports these format names for whole container families.
const (
	probeMatroska = "matroska,webm"
	probeISOBMFF  = "mov,mp4,m4a,3gp,3g2,mj2"
	probeMPEGTS   = "mpegts"
	probeMPEGPS   = "mpeg"
)

// Fixed sizes and rates some legacy codecs require.
const (
	// cifWidth and cifHeight are the CIF size; H.263 only encodes a few fixed sizes.
	cifWidth  = 352
	cifHeight = 288
	// realAudio144SampleRate is the only rate the RealAudio 1.0 (14.4K) encoder accepts.
	realAudio144SampleRate = 8000
	monoChannels           = 1
)

const (
	pixelFormat8Bit  = "yuv420p"
	pixelFormat10Bit = "yuv420p10le"
	// hdr10X265Params adds HDR10 mastering display (BT.2020 primaries, 1000 nits peak) and
	// content light level SEI. log-level keeps x265 from printing banners to stderr.
	hdr10X265Params = "log-level=error:hdr10=1:repeat-headers=1:" +
		"master-display=G(13250,34500)B(7500,3000)R(34000,16000)WP(15635,16450)L(10000000,1):" +
		"max-cll=1000,400"
	// hdr10ColorTags tags the frames as BT.2020/PQ before encoding. FFmpeg 9 drops the
	// -color_primaries/-color_trc encoder options in this pipeline, the filter does not.
	hdr10ColorTags = "setparams=color_primaries=bt2020:color_trc=smpte2084:colorspace=bt2020nc"
)

// videoCases returns the video container and codec cases.
func videoCases() []mediaCase {
	return slices.Concat(
		matroskaCases(),
		hdrCases(),
		isoMediaCases(),
		aviCases(),
		transportStreamCases(),
		webCases(),
		av1Cases(),
		programStreamCases(),
		legacyCases(),
		realMediaCases(),
	)
}

func matroskaCases() []mediaCase {
	return []mediaCase{
		{
			Name: "video-mkv-h264-aac", Category: categoryVideo, Output: "video/h264_aac.mkv",
			Source: standardSource(), Muxer: "matroska", Encoders: []string{"libx264", "aac"},
			Args:   slices.Concat(x264Args(), audioArgs("aac")),
			Expect: expect("mkv", probeMatroska, h264Video(), audioOf("aac")),
		},
		{
			Name: "video-mkv-hevc10-ac3", Category: categoryVideo, Output: "video/hevc_main10_ac3.mkv",
			Source: standardSource(), Muxer: "matroska", Encoders: []string{"libx265", "ac3"},
			Args: slices.Concat(
				x265Main10Args("log-level=error"), audioArgs("ac3")),
			Expect: expect("mkv", probeMatroska, hevcMain10Video(), audioOf("ac3")),
		},
	}
}

func hdrCases() []mediaCase {
	video := hevcMain10Video()
	video.ColorPrimaries = "bt2020"
	video.ColorTransfer = "smpte2084"
	video.ColorSpace = "bt2020nc"
	video.HDR = "hdr10"

	return []mediaCase{
		{
			Name: "video-mkv-hevc10-hdr10-ac3", Category: categoryVideo,
			Output: "video/hevc_main10_hdr10_ac3.mkv",
			Source: standardSource(), Muxer: "matroska", Encoders: []string{"libx265", "ac3"},
			Args: slices.Concat(
				[]string{"-vf", hdr10ColorTags},
				x265Main10Args(hdr10X265Params), audioArgs("ac3")),
			Expect: expect("mkv", probeMatroska, video, audioOf("ac3")),
		},
	}
}

func isoMediaCases() []mediaCase {
	return []mediaCase{
		{
			Name: "video-mp4-h264-aac-faststart", Category: categoryVideo,
			Output: "video/h264_aac_faststart.mp4",
			Source: standardSource(), Muxer: "mp4", Encoders: []string{"libx264", "aac"},
			Args:   slices.Concat(x264Args(), audioArgs("aac"), []string{"-movflags", "+faststart"}),
			Expect: expect("mp4", probeISOBMFF, h264Video(), audioOf("aac")),
		},
		{
			// The ipod muxer writes the MP4 flavour Apple uses for .m4v files.
			Name: "video-m4v-h264-aac", Category: categoryVideo, Output: "video/h264_aac.m4v",
			Source: standardSource(), Muxer: "ipod", Encoders: []string{"libx264", "aac"},
			Args:   slices.Concat(x264Args(), audioArgs("aac")),
			Expect: expect("m4v", probeISOBMFF, h264Video(), audioOf("aac")),
		},
		{
			Name: "video-mov-h264-pcm", Category: categoryVideo, Output: "video/h264_pcm.mov",
			Source: standardSource(), Muxer: "mov", Encoders: []string{"libx264", "pcm_s16le"},
			Args:   slices.Concat(x264Args(), audioArgs("pcm_s16le")),
			Expect: expect("mov", probeISOBMFF, h264Video(), audioOf("pcm_s16le")),
		},
		{
			Name: "video-3gp-h263-aac", Category: categoryVideo, Output: "video/h263_aac.3gp",
			Source: standardSource().withSize(cifWidth, cifHeight), Muxer: "3gp",
			Encoders: []string{"h263", "aac"},
			Args:     slices.Concat(simpleVideoArgs("h263"), audioArgs("aac")),
			Expect:   expect("3gp", probeISOBMFF, videoOf("h263", pixelFormat8Bit), audioOf("aac")),
		},
	}
}

func aviCases() []mediaCase {
	return []mediaCase{
		mpeg4AVICase("video-avi-xvid-mp3", "video/mpeg4_xvid_mp3.avi", "XVID"),
		mpeg4AVICase("video-avi-divx-mp3", "video/mpeg4_divx_mp3.avi", "DIVX"),
	}
}

// mpeg4AVICase builds an MPEG-4 Part 2 AVI whose FourCC claims the given encoder family;
// players and scanners use the tag to label files as Xvid or DivX.
func mpeg4AVICase(name, output, fourCC string) mediaCase {
	video := videoOf("mpeg4", pixelFormat8Bit)
	video.CodecTag = fourCC

	return mediaCase{
		Name: name, Category: categoryVideo, Output: output,
		Source: standardSource(), Muxer: "avi", Encoders: []string{"mpeg4", "libmp3lame"},
		Args: slices.Concat(
			simpleVideoArgs("mpeg4"), []string{"-vtag", fourCC}, audioArgs("libmp3lame")),
		Expect: expect("avi", "avi", video, audioOf("mp3")),
	}
}

func transportStreamCases() []mediaCase {
	// -mpegts_m2ts_mode writes the 192-byte packets of Blu-ray (.m2ts) and AVCHD (.mts).
	m2tsArgs := slices.Concat(x264Args(), audioArgs("ac3"), []string{"-mpegts_m2ts_mode", "1"})

	return []mediaCase{
		{
			Name: "video-ts-mpeg2-mp2", Category: categoryVideo, Output: "video/mpeg2_mp2.ts",
			Source: standardSource(), Muxer: "mpegts", Encoders: []string{"mpeg2video", "mp2"},
			Args:   slices.Concat(simpleVideoArgs("mpeg2video"), audioArgs("mp2")),
			Expect: expect("mpegts", probeMPEGTS, mpeg2Video(), audioOf("mp2")),
		},
		{
			Name: "video-m2ts-h264-ac3", Category: categoryVideo, Output: "video/h264_ac3.m2ts",
			Source: standardSource(), Muxer: "mpegts", Encoders: []string{"libx264", "ac3"},
			Args:   m2tsArgs,
			Expect: expect("m2ts", probeMPEGTS, h264Video(), audioOf("ac3")),
		},
		{
			Name: "video-mts-h264-ac3", Category: categoryVideo, Output: "video/h264_ac3.mts",
			Source: standardSource(), Muxer: "mpegts", Encoders: []string{"libx264", "ac3"},
			Args:   m2tsArgs,
			Expect: expect("m2ts", probeMPEGTS, h264Video(), audioOf("ac3")),
		},
	}
}

func webCases() []mediaCase {
	return []mediaCase{
		{
			Name: "video-webm-vp9-opus", Category: categoryVideo, Output: "video/vp9_opus.webm",
			Source: standardSource(), Muxer: "webm", Encoders: []string{"libvpx-vp9", "libopus"},
			Args: slices.Concat(
				simpleVideoArgs("libvpx-vp9"),
				[]string{"-deadline", "realtime", "-cpu-used", "8", "-crf", "40", "-b:v", "0"},
				audioArgs("libopus")),
			Expect: expect("webm", probeMatroska, videoOf("vp9", pixelFormat8Bit), audioOf("opus")),
		},
		{
			Name: "video-flv-h264-aac", Category: categoryVideo, Output: "video/h264_aac.flv",
			Source: standardSource(), Muxer: "flv", Encoders: []string{"libx264", "aac"},
			Args:   slices.Concat(x264Args(), audioArgs("aac")),
			Expect: expect("flv", "flv", h264Video(), audioOf("aac")),
		},
		{
			Name: "video-ogv-theora-vorbis", Category: categoryVideo, Output: "video/theora_vorbis.ogv",
			Source: standardSource(), Muxer: "ogv", Encoders: []string{"libtheora", "libvorbis"},
			Args:   slices.Concat(simpleVideoArgs("libtheora"), audioArgs("libvorbis")),
			Expect: expect("ogg", "ogg", videoOf("theora", pixelFormat8Bit), audioOf("vorbis")),
		},
	}
}

// av1Cases lists one WebM AV1 case per software encoder, in order of preference; the first
// encoder the local FFmpeg has wins. All are tuned for speed, the clip only has to decode.
func av1Cases() []mediaCase {
	encoders := []struct {
		name string
		args []string
	}{
		{name: "libsvtav1", args: []string{"-preset", "12"}},
		{name: "libaom-av1", args: []string{"-cpu-used", "8", "-row-mt", "1"}},
		{name: "librav1e", args: []string{"-speed", "10"}},
	}

	cases := make([]mediaCase, 0, len(encoders))
	for _, encoder := range encoders {
		cases = append(cases, mediaCase{
			Name: "video-webm-av1-opus", Category: categoryVideo, Output: "video/av1_opus.webm",
			Source: standardSource(), Muxer: "webm", Encoders: []string{encoder.name, "libopus"},
			Args: slices.Concat(
				simpleVideoArgs(encoder.name), encoder.args, audioArgs("libopus")),
			Expect: expect("webm", probeMatroska, videoOf("av1", pixelFormat8Bit), audioOf("opus")),
		})
	}

	return cases
}

func programStreamCases() []mediaCase {
	return []mediaCase{
		{
			Name: "video-mpg-mpeg2-mp2", Category: categoryVideo, Output: "video/mpeg2_mp2.mpg",
			Source: standardSource(), Muxer: "mpeg", Encoders: []string{"mpeg2video", "mp2"},
			Args:   slices.Concat(simpleVideoArgs("mpeg2video"), audioArgs("mp2")),
			Expect: expect("mpeg", probeMPEGPS, mpeg2Video(), audioOf("mp2")),
		},
		{
			Name: "video-vob-mpeg2-ac3", Category: categoryVideo, Output: "video/mpeg2_ac3.vob",
			Source: standardSource(), Muxer: "vob", Encoders: []string{"mpeg2video", "ac3"},
			Args:   slices.Concat(simpleVideoArgs("mpeg2video"), audioArgs("ac3")),
			Expect: expect("vob", probeMPEGPS, mpeg2Video(), audioOf("ac3")),
		},
	}
}

func legacyCases() []mediaCase {
	return []mediaCase{
		{
			Name: "video-wmv-wmv2-wmav2", Category: categoryVideo, Output: "video/wmv2_wmav2.wmv",
			Source: standardSource(), Muxer: "asf", Encoders: []string{"wmv2", "wmav2"},
			Args:   slices.Concat(simpleVideoArgs("wmv2"), audioArgs("wmav2")),
			Expect: expect("asf", "asf", videoOf("wmv2", pixelFormat8Bit), audioOf("wmav2")),
		},
		{
			Name: "video-mxf-mpeg2-pcm", Category: categoryVideo, Output: "video/mpeg2_pcm.mxf",
			Source: standardSource(), Muxer: "mxf", Encoders: []string{"mpeg2video", "pcm_s16le"},
			Args:   slices.Concat(simpleVideoArgs("mpeg2video"), audioArgs("pcm_s16le")),
			Expect: expect("mxf", "mxf", mpeg2Video(), audioOf("pcm_s16le")),
		},
	}
}

// realMediaCases is best effort: FFmpeg only encodes RealVideo 1.0/2.0, while typical RMVB
// files use RealVideo 4.0 (a known gap, ADR 0021). The .rmvb file still exercises the
// extension and the demuxer.
func realMediaCases() []mediaCase {
	source := standardSource().withAudio(realAudio144SampleRate, monoChannels)
	args := slices.Concat(simpleVideoArgs("rv20"), audioArgs("real_144"))
	expected := expect("rm", "rm", videoOf("rv20", pixelFormat8Bit), audioOf("ra_144"))

	return []mediaCase{
		{
			Name: "video-rm-rv20-ra144", Category: categoryVideo, Output: "video/rv20_ra144.rm",
			Source: source, Muxer: "rm", Encoders: []string{"rv20", "real_144"},
			Args: args, Expect: expected,
		},
		{
			Name: "video-rmvb-rv20-ra144", Category: categoryVideo, Output: "video/rv20_ra144.rmvb",
			Source: source, Muxer: "rm", Encoders: []string{"rv20", "real_144"},
			Args: args, Expect: expected,
		},
		knownGap("video-rmvb-rv40-cook", "video/rv40_cook.rmvb",
			expect("rm", "rm", &videoExpectation{Codec: "rv40"}, audioOf("cook")),
			"ffmpeg has no RealVideo 4 (RV40) or Cook encoder"),
	}
}

// x264Args encodes 8-bit 4:2:0 H.264 High profile, the format every browser plays.
func x264Args() []string {
	return []string{"-c:v", "libx264", "-preset", "veryfast", "-profile:v", "high", "-pix_fmt", pixelFormat8Bit}
}

// x265Main10Args encodes 10-bit HEVC Main10 with the given x265 parameters.
func x265Main10Args(x265Params string) []string {
	return []string{
		"-c:v", "libx265", "-preset", "ultrafast", "-profile:v", "main10",
		"-pix_fmt", pixelFormat10Bit, "-x265-params", x265Params,
	}
}

// simpleVideoArgs selects an encoder with 8-bit 4:2:0 output and its default settings.
func simpleVideoArgs(encoder string) []string {
	return []string{"-c:v", encoder, "-pix_fmt", pixelFormat8Bit}
}

func audioArgs(encoder string) []string {
	return []string{"-c:a", encoder}
}

// expect builds the expectation of a file with at most one video and one audio stream.
func expect(container, probeFormat string, video *videoExpectation, audio *audioExpectation) expectation {
	result := expectation{Container: container, ProbeFormat: probeFormat, Video: video}
	if audio != nil {
		result.Audio = []audioExpectation{*audio}
	}

	return result
}

func videoOf(codec, pixelFormat string) *videoExpectation {
	return &videoExpectation{Codec: codec, PixelFormat: pixelFormat}
}

func audioOf(codec string) *audioExpectation {
	return &audioExpectation{Codec: codec}
}

func h264Video() *videoExpectation {
	video := videoOf("h264", pixelFormat8Bit)
	video.Profile = "High"

	return video
}

func hevcMain10Video() *videoExpectation {
	video := videoOf("hevc", pixelFormat10Bit)
	video.Profile = "Main 10"

	return video
}

func mpeg2Video() *videoExpectation {
	return videoOf("mpeg2video", pixelFormat8Bit)
}

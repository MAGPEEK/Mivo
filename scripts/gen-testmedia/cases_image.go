package main

import (
	"slices"
	"time"
)

const (
	// imageSourceDuration bounds the lavfi source; the frame limit decides the image length.
	imageSourceDuration = time.Second
	stillFrames         = 1
	// animationFrames at animationFrameRate give a one-second animated GIF.
	animationFrames    = 10
	animationFrameRate = 10

	pixelFormatRGB = "rgb24"
	// image2Muxer writes single image files; -update 1 tells it the name has no frame number.
	image2Muxer = "image2"
	// probeImage2 is the format ffprobe reports for JPEG files.
	probeImage2 = "image2"
)

// imageCases returns still and animated image cases and the image formats FFmpeg cannot write.
func imageCases() []mediaCase {
	return slices.Concat(
		stillImageCases(),
		animatedImageCases(),
		avifCases(),
		imageGaps(),
	)
}

// imageSource is a 320x240 test pattern limited to the given number of frames.
func imageSource(frameRate, frames int) sourceSpec {
	return sourceSpec{
		Width: defaultWidth, Height: defaultHeight, FrameRate: frameRate,
		Frames: frames, Duration: imageSourceDuration,
	}
}

func stillImageCases() []mediaCase {
	updateSingleFile := []string{"-update", "1"}

	return []mediaCase{
		{
			// The JPEG pixel format (yuvj420p or yuv420p with full range) differs between
			// FFmpeg versions, so it is not part of the expectation.
			Name: "image-jpeg", Category: categoryImage, Output: "image/mjpeg.jpg",
			Source: imageSource(defaultFrameRate, stillFrames), Muxer: image2Muxer, Encoders: []string{"mjpeg"},
			Args:   slices.Concat([]string{"-c:v", "mjpeg"}, updateSingleFile),
			Expect: expect("jpeg", probeImage2, &videoExpectation{Codec: "mjpeg"}, nil),
		},
		{
			Name: "image-png", Category: categoryImage, Output: "image/png.png",
			Source: imageSource(defaultFrameRate, stillFrames), Muxer: image2Muxer, Encoders: []string{"png"},
			Args:   slices.Concat([]string{"-c:v", "png", "-pix_fmt", pixelFormatRGB}, updateSingleFile),
			Expect: expect("png", "png_pipe", videoOf("png", pixelFormatRGB), nil),
		},
		{
			// RGB rather than FFmpeg's YCbCr default: most TIFF readers only handle RGB.
			Name: "image-tiff", Category: categoryImage, Output: "image/tiff.tiff",
			Source: imageSource(defaultFrameRate, stillFrames), Muxer: image2Muxer, Encoders: []string{"tiff"},
			Args:   slices.Concat([]string{"-c:v", "tiff", "-pix_fmt", pixelFormatRGB}, updateSingleFile),
			Expect: expect("tiff", "tiff_pipe", videoOf("tiff", pixelFormatRGB), nil),
		},
		{
			// libwebp encodes lossy unless -lossless 1 is given.
			Name: "image-webp-lossy", Category: categoryImage, Output: "image/webp_lossy.webp",
			Source: imageSource(defaultFrameRate, stillFrames), Muxer: "webp", Encoders: []string{"libwebp"},
			Args:   []string{"-c:v", "libwebp", "-lossless", "0", "-pix_fmt", pixelFormat8Bit},
			Expect: expect("webp", "webp_pipe", videoOf("webp", pixelFormat8Bit), nil),
		},
	}
}

func animatedImageCases() []mediaCase {
	return []mediaCase{
		{
			// The GIF encoder maps the frames to its default palette.
			Name: "image-gif-animated", Category: categoryImage, Output: "image/gif_animated.gif",
			Source: imageSource(animationFrameRate, animationFrames), Muxer: "gif", Encoders: []string{"gif"},
			Args:   []string{"-c:v", "gif"},
			Expect: expect("gif", "gif", &videoExpectation{Codec: "gif"}, nil),
		},
	}
}

// avifCases lists one AVIF still per AV1 encoder, in order of preference: libaom has the most
// complete still-picture support.
func avifCases() []mediaCase {
	encoders := []struct {
		name string
		args []string
	}{
		{name: "libaom-av1", args: []string{"-still-picture", "1", "-cpu-used", "8"}},
		{name: "libsvtav1", args: []string{"-preset", "12"}},
		{name: "librav1e", args: []string{"-speed", "10"}},
	}

	cases := make([]mediaCase, 0, len(encoders))
	for _, encoder := range encoders {
		cases = append(cases, mediaCase{
			Name: "image-avif", Category: categoryImage, Output: "image/av1.avif",
			Source: imageSource(defaultFrameRate, stillFrames), Muxer: "avif", Encoders: []string{encoder.name},
			Args:   slices.Concat(simpleVideoArgs(encoder.name), encoder.args),
			Expect: expect("avif", probeISOBMFF, videoOf("av1", pixelFormat8Bit), nil),
		})
	}

	return cases
}

// imageGaps lists photo formats and metadata FFmpeg cannot write (ADR 0021).
func imageGaps() []mediaCase {
	return []mediaCase{
		knownGap("image-heic", "image/hevc.heic",
			expect("heic", probeISOBMFF, &videoExpectation{Codec: "hevc"}, nil),
			"ffmpeg has no HEIF/HEIC image muxer"),
		knownGap("image-jpeg-exif", "image/mjpeg_exif.jpg",
			expect("jpeg", probeImage2, &videoExpectation{Codec: "mjpeg"}, nil),
			"ffmpeg cannot write EXIF metadata (camera, date, GPS, orientation) into JPEG files"),
	}
}

package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// Configuration keys of the external tools, named in startup messages.
const (
	ffmpegSetting  = "tools.ffmpeg"
	ffprobeSetting = "tools.ffprobe"
)

// windowsOS is runtime.GOOS on Windows.
const windowsOS = "windows"

// missingToolsDirectory names the never-created directory below the data directory that
// unavailableToolPath points into.
const missingToolsDirectory = "missing-tools"

// windowsIgnoredNameSuffix are the characters Windows drops from the end of a file name, so
// "ffmpeg.bat." and "ffmpeg.bat " still run as batch scripts.
const windowsIgnoredNameSuffix = ". "

var (
	// errToolMissing marks a tool that is not installed where the configuration says. The
	// server still starts: direct play needs no FFmpeg, and installing it later only needs a
	// restart.
	errToolMissing = errors.New("not found")
	// errToolNotRegularFile marks a configured tool that resolves to a directory or a device.
	errToolNotRegularFile = errors.New("is not a regular file")
	// errToolIsBatchScript marks a Windows batch file: cmd.exe parses its command line again,
	// so a media file name such as "x&calc&.mkv" could run commands (M2-11 review).
	errToolIsBatchScript = errors.New("is a batch script, which cmd.exe would re-parse; configure the .exe")
)

// windowsBatchExtensions are the extensions cmd.exe runs as scripts.
func windowsBatchExtensions() []string {
	return []string{".bat", ".cmd"}
}

// toolHost resolves tool names the way os/exec does; tests inject the OS and the lookup.
type toolHost struct {
	goos     string
	lookPath func(file string) (string, error)
}

// externalTools are the FFmpeg executables handed to the adapters and what ffmpeg can do.
type externalTools struct {
	// ffmpeg and ffprobe are absolute paths; for a missing tool a path that never exists
	// (unavailableToolPath), so the adapters fail with domain.ErrUnavailable when they run it.
	ffmpeg       string
	ffprobe      string
	capabilities playback.ServerCapabilities
}

// prepareTools resolves both tools once at startup and detects the capabilities of ffmpeg.
// A missing tool only degrades the server (warning); a tool that is configured but unusable,
// such as a directory or a batch script, fails the start.
func (app application) prepareTools(ctx context.Context) (externalTools, error) {
	ffprobe, ffprobeFound, err := app.resolveConfiguredTool(ctx, ffprobeSetting, app.config.Tools.FFprobe)
	if err != nil {
		return externalTools{}, err
	}

	if !ffprobeFound {
		app.logger.WarnContext(ctx, "ffprobe not found: library scans fail until it is installed and the server restarted",
			slog.String("setting", ffprobeSetting), slog.String("value", app.config.Tools.FFprobe))
	}

	ffmpegPath, ffmpegFound, err := app.resolveConfiguredTool(ctx, ffmpegSetting, app.config.Tools.FFmpeg)
	if err != nil {
		return externalTools{}, err
	}

	capabilities, err := app.detectCapabilities(ctx, ffmpegPath, ffmpegFound)
	if err != nil {
		return externalTools{}, err
	}

	return externalTools{ffmpeg: ffmpegPath, ffprobe: ffprobe, capabilities: capabilities}, nil
}

// resolveConfiguredTool returns the absolute path of a tool and true, or unavailableToolPath
// and false when the tool is missing.
func (app application) resolveConfiguredTool(ctx context.Context, setting, value string) (string, bool, error) {
	path, err := resolveTool(setting, value, app.tools)
	if errors.Is(err, errToolMissing) {
		app.logger.DebugContext(ctx, "tool lookup failed", slog.String("setting", setting), slog.Any("error", err))

		return app.unavailableToolPath(value), false, nil
	}

	if err != nil {
		return "", false, err
	}

	app.logger.InfoContext(ctx, "tool found", slog.String("setting", setting), slog.String("path", path))

	return path, true, nil
}

// unavailableToolPath is what the adapters get for a missing tool: an absolute path below
// <data>/missingToolsDirectory, which the server never creates. Handing them the configured
// bare name instead would let os/exec repeat the PATH lookup on every run and start a tool
// installed later without the startup checks (batch script, regular file); this path fails
// every run with domain.ErrUnavailable until the server is restarted.
func (app application) unavailableToolPath(value string) string {
	return filepath.Join(app.config.Paths.Data, missingToolsDirectory, filepath.Base(value))
}

// resolveTool resolves value with the PATH lookup of os/exec, which also adds the Windows
// executable extensions to a path without one, and checks the result: it must be a regular
// file and, on Windows, no batch script. A tool that does not exist wraps errToolMissing.
func resolveTool(setting, value string, host toolHost) (string, error) {
	path, err := host.lookPath(value)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("%s %q: %w: %w", setting, value, errToolMissing, err)
		}

		return "", fmt.Errorf("%s %q: %w", setting, value, err)
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("%s %q: make %s absolute: %w", setting, value, path, err)
	}

	if err := checkToolFile(absolute, host.goos); err != nil {
		return "", fmt.Errorf("%s %q: %w", setting, value, err)
	}

	return absolute, nil
}

// checkToolFile refuses batch scripts on Windows and anything but a regular file.
func checkToolFile(path, goos string) error {
	if goos == windowsOS && isBatchScriptName(path) {
		return fmt.Errorf("%s %w", path, errToolIsBatchScript)
	}

	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%s: %w", path, errToolMissing)
	}

	if err != nil {
		return fmt.Errorf("inspect %s: %w", path, err)
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s %w", path, errToolNotRegularFile)
	}

	return nil
}

// detectCapabilities asks ffmpeg what it can do. Without a usable ffmpeg the capabilities
// stay zero, so the decision engine offers direct play only and answers transcode_unavailable
// for everything else. Only a shutdown during the detection and a wiring error are errors.
func (app application) detectCapabilities(ctx context.Context, ffmpegPath string, found bool) (playback.ServerCapabilities, error) {
	if !found {
		app.logger.WarnContext(ctx, "ffmpeg not found: only direct play available",
			slog.String("setting", ffmpegSetting), slog.String("value", app.config.Tools.FFmpeg))

		return playback.ServerCapabilities{}, nil
	}

	capabilities, err := ffmpeg.DetectCapabilities(ctx, ffmpegPath, app.logger)
	// A shutdown, or ErrInvalid for a wiring mistake such as a nil logger, fails the start;
	// only an FFmpeg that cannot answer degrades the server.
	if err != nil && (ctx.Err() != nil || errors.Is(err, domain.ErrInvalid)) {
		return playback.ServerCapabilities{}, fmt.Errorf("detect ffmpeg capabilities: %w", err)
	}

	// The adapter has logged the failure with FFmpeg's error output (ADR 0024 Notes M2-39).
	if err != nil {
		return playback.ServerCapabilities{}, nil
	}

	app.logCapabilities(ctx, capabilities)

	return capabilities, nil
}

// logCapabilities writes one line with the version and the transcode encoders, at warn when
// a playback mode is unavailable.
func (app application) logCapabilities(ctx context.Context, capabilities playback.ServerCapabilities) {
	attributes := []any{
		slog.String("version", capabilities.FFmpegVersion),
		slog.Bool("hls_muxing", capabilities.HLSMuxing),
		slog.String("h264_encoder", capabilities.H264Encoder),
		slog.String("aac_encoder", capabilities.AACEncoder),
	}

	switch {
	case !capabilities.HLSMuxing:
		app.logger.WarnContext(ctx, "ffmpeg cannot write HLS: only direct play available", attributes...)
	case capabilities.H264Encoder == "" || capabilities.AACEncoder == "":
		app.logger.WarnContext(ctx, "ffmpeg lacks the H.264 or AAC encoder: direct play and remux only, no transcoding", attributes...)
	default:
		app.logger.InfoContext(ctx, "ffmpeg ready for direct play, remux and transcode", attributes...)
	}
}

// isBatchScriptName reports a name cmd.exe runs as a script, also with the trailing dots or
// spaces Windows ignores.
func isBatchScriptName(path string) bool {
	extension := filepath.Ext(strings.TrimRight(path, windowsIgnoredNameSuffix))

	return slices.Contains(windowsBatchExtensions(), strings.ToLower(extension))
}

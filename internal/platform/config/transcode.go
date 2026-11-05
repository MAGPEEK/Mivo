package config

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
)

// Defaults of the transcoder settings (ADR 0024, ADR 0030 §1). They keep a 2-core NAS
// responsive: a fast preset, low OS priority and a bounded lead over the player.
const (
	// DefaultTranscodeMaxSessions follows OD-4: two re-encoding sessions saturate a typical
	// NAS CPU. Small machines get SmallMachineTranscodeMaxSessions instead.
	DefaultTranscodeMaxSessions = 2
	// SmallMachineTranscodeMaxSessions is the default on machines with at most
	// SmallMachineCPUCount logical CPUs, where a second x264 encode starves the first.
	SmallMachineTranscodeMaxSessions = 1
	// SmallMachineCPUCount is the largest CPU count that counts as a small machine.
	SmallMachineCPUCount = 4

	DefaultTranscodePreset = PresetVeryfast
	// DefaultTranscodeThreads 0 lets the transcoder choose (ADR 0030: max(1, NumCPU/2)).
	DefaultTranscodeThreads              = 0
	DefaultTranscodePriority             = PriorityLow
	DefaultTranscodeThrottleAheadSeconds = 60
	DefaultTranscodeHWAccel              = HWAccelAuto
	DefaultTranscodeAllowBurnIn          = true
)

// x264 presets the server accepts, fastest first. Slower presets than medium cost more CPU
// than a NAS can spend on live playback (ADR 0030 §1).
const (
	PresetUltrafast = "ultrafast"
	PresetSuperfast = "superfast"
	PresetVeryfast  = "veryfast"
	PresetFaster    = "faster"
	PresetFast      = "fast"
	PresetMedium    = "medium"
)

// OS scheduling priorities of FFmpeg processes.
const (
	PriorityLow    = "low"
	PriorityNormal = "normal"
)

// Hardware acceleration modes (ADR 0030 §5).
const (
	HWAccelAuto  = "auto"
	HWAccelOff   = "off"
	HWAccelVAAPI = "vaapi"
	HWAccelQSV   = "qsv"
	HWAccelNVENC = "nvenc"
	HWAccelAMF   = "amf"
)

const (
	keyTranscodeMaxSessions   = "transcode.max_sessions"
	keyTranscodePreset        = "transcode.preset"
	keyTranscodeThreads       = "transcode.threads"
	keyTranscodePriority      = "transcode.priority"
	keyTranscodeThrottleAhead = "transcode.throttle_ahead_seconds"
	keyTranscodeHWAccel       = "transcode.hwaccel"
	keyTranscodeHWAccelDevice = "transcode.hwaccel_device"
	keyTranscodeAllowBurnIn   = "transcode.allow_burn_in"
)

const (
	// The transcode limit counts re-encoding sessions only (ADR 0024, OD-4). Its maximum is
	// the fixed safety cap of 8 FFmpeg sessions of ADR 0024: a higher limit could never be
	// reached, so it would be silently ineffective.
	minTranscodeSessions = 1
	maxTranscodeSessions = 8

	// maxTranscodeThreads bounds the thread count; x264 gains nothing beyond it.
	maxTranscodeThreads = 64

	// throttleOff turns throttling off; any other lead must lie in the range below, which
	// keeps enough buffer for seeking yet bounds the transcode directory (ADR 0030 §2).
	throttleOff              = 0
	minThrottleAheadSeconds  = 20
	maxThrottleAheadSeconds  = 600
	transcodeThreadsAutoHint = "use 0 for auto"
)

// errHWAccelDeviceNotAbsolute reports a relative hardware acceleration device.
var errHWAccelDeviceNotAbsolute = errors.New("is not an absolute path; leave it empty for the default device")

// Transcode holds the limits and tuning of FFmpeg sessions (ADR 0024, ADR 0030).
type Transcode struct {
	// MaxSessions limits concurrent playback sessions that re-encode a stream; remux
	// sessions only count towards the fixed safety cap of the playback service.
	MaxSessions int
	// Preset is one of the Preset* constants.
	Preset string
	// Threads is the encoder thread count; 0 lets the transcoder choose.
	Threads int
	// Priority is one of the Priority* constants.
	Priority string
	// ThrottleAheadSeconds pauses FFmpeg when it is this far ahead of the player; 0 turns
	// throttling off.
	ThrottleAheadSeconds int
	// HWAccel is one of the HWAccel* constants.
	HWAccel string
	// HWAccelDevice is an absolute device path such as /dev/dri/renderD128; empty selects
	// the default device. Whether it works is checked by the hardware test, not here.
	HWAccelDevice string
	// AllowBurnIn permits burning subtitles into a re-encoded video.
	AllowBurnIn bool
}

// cpuCount returns the CPU count from the injected function, else from the runtime.
func cpuCount(options Options) int {
	if options.NumCPU != nil {
		return options.NumCPU()
	}

	return runtime.NumCPU()
}

func defaultTranscode(cpuCount int) Transcode {
	maxSessions := DefaultTranscodeMaxSessions
	if cpuCount <= SmallMachineCPUCount {
		maxSessions = SmallMachineTranscodeMaxSessions
	}

	return Transcode{
		MaxSessions:          maxSessions,
		Preset:               DefaultTranscodePreset,
		Threads:              DefaultTranscodeThreads,
		Priority:             DefaultTranscodePriority,
		ThrottleAheadSeconds: DefaultTranscodeThrottleAheadSeconds,
		HWAccel:              DefaultTranscodeHWAccel,
		AllowBurnIn:          DefaultTranscodeAllowBurnIn,
	}
}

// transcodeSettings lists the keys of the transcode section.
func transcodeSettings() []setting {
	return []setting{
		{key: keyTranscodeMaxSessions, apply: applyMaxSessions},
		{
			key: keyTranscodePreset,
			apply: applyChoice(func(cfg *Config) *string { return &cfg.Transcode.Preset },
				PresetUltrafast, PresetSuperfast, PresetVeryfast, PresetFaster, PresetFast, PresetMedium),
		},
		{key: keyTranscodeThreads, apply: applyThreads},
		{
			key:   keyTranscodePriority,
			apply: applyChoice(func(cfg *Config) *string { return &cfg.Transcode.Priority }, PriorityLow, PriorityNormal),
		},
		{key: keyTranscodeThrottleAhead, apply: applyThrottleAhead},
		{
			key: keyTranscodeHWAccel,
			apply: applyChoice(func(cfg *Config) *string { return &cfg.Transcode.HWAccel },
				HWAccelAuto, HWAccelOff, HWAccelVAAPI, HWAccelQSV, HWAccelNVENC, HWAccelAMF),
		},
		{key: keyTranscodeHWAccelDevice, apply: applyHWAccelDevice},
		{key: keyTranscodeAllowBurnIn, apply: applyBool(func(cfg *Config) *bool { return &cfg.Transcode.AllowBurnIn })},
	}
}

func applyMaxSessions(cfg *Config, raw, _ string) error {
	sessions, err := parseIntInRange("re-encoding session limit", raw, minTranscodeSessions, maxTranscodeSessions)
	if err != nil {
		return fmt.Errorf("%w; the server runs at most %d FFmpeg sessions at once", err, maxTranscodeSessions)
	}

	cfg.Transcode.MaxSessions = sessions

	return nil
}

func applyThreads(cfg *Config, raw, _ string) error {
	threads, err := parseIntInRange("thread count", raw, DefaultTranscodeThreads, maxTranscodeThreads)
	if err != nil {
		return fmt.Errorf("%w; %s", err, transcodeThreadsAutoHint)
	}

	cfg.Transcode.Threads = threads

	return nil
}

// applyThrottleAhead accepts 0 (off) or a lead within the allowed range.
func applyThrottleAhead(cfg *Config, raw, _ string) error {
	seconds, err := parseThrottleAhead(raw)
	if err != nil {
		return fmt.Errorf("%w; use 0 to turn throttling off", err)
	}

	cfg.Transcode.ThrottleAheadSeconds = seconds

	return nil
}

func parseThrottleAhead(raw string) (int, error) {
	seconds, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%w: throttle lead %q is not a number of seconds", ErrInvalid, raw)
	}

	if seconds != throttleOff && (seconds < minThrottleAheadSeconds || seconds > maxThrottleAheadSeconds) {
		return 0, fmt.Errorf("%w: throttle lead %d seconds is outside %d-%d",
			ErrInvalid, seconds, minThrottleAheadSeconds, maxThrottleAheadSeconds)
	}

	return seconds, nil
}

// applyHWAccelDevice accepts an empty value or an absolute path. A slash-rooted path counts
// as absolute on every host, because device nodes are Linux paths even when the
// configuration is checked on a Windows development machine.
func applyHWAccelDevice(cfg *Config, raw, _ string) error {
	if raw != "" && !path.IsAbs(raw) && !filepath.IsAbs(raw) {
		return fmt.Errorf("%w: %q %w", ErrInvalid, raw, errHWAccelDeviceNotAbsolute)
	}

	cfg.Transcode.HWAccelDevice = raw

	return nil
}

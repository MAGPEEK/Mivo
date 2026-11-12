import type { SessionSummary, TranscoderDevice, TranscoderStatus } from "../api/transcoderApi";

/** What the test found for one hardware encoder. */
export type DeviceResult = "usable" | "degraded" | "unavailable";

/** A degraded encoder is not used, even when its test encode passed. */
export function deviceResult(device: TranscoderDevice): DeviceResult {
  if (device.degraded) {
    return "degraded";
  }
  return device.usable ? "usable" : "unavailable";
}

/** Whether any tested encoder can be used; false means software encoding. */
export function hasUsableDevice(status: TranscoderStatus): boolean {
  return status.devices.some((device) => deviceResult(device) === "usable");
}

/** The status panel shows software encoding when no test result lists a usable encoder. */
export function isSoftwareOnly(status: TranscoderStatus): boolean {
  return !hasUsableDevice(status);
}

/** A bit rate split into a unit key and a rounded value, ready for a translated unit. */
export interface BitRateParts {
  readonly unit: "mbit" | "kbit";
  readonly value: number;
}

const bitsPerKilobit = 1000;
const bitsPerMegabit = 1_000_000;
const tenth = 10;

function roundToTenth(value: number): number {
  return Math.round(value * tenth) / tenth;
}

/** Bit rate in bits per second as Mbit/s, or kbit/s below one megabit; null when unknown. */
export function bitRateParts(bitsPerSecond: number | undefined): BitRateParts | null {
  if (bitsPerSecond === undefined) {
    return null;
  }
  if (bitsPerSecond >= bitsPerMegabit) {
    return { unit: "mbit", value: roundToTenth(bitsPerSecond / bitsPerMegabit) };
  }
  return { unit: "kbit", value: Math.round(bitsPerSecond / bitsPerKilobit) };
}

/** Transcode speed rounded to one decimal; null when FFmpeg reported none. */
export function speedValue(speed: number | undefined): number | null {
  return speed === undefined ? null : roundToTenth(speed);
}

/** Whether the client will stall because FFmpeg is slower than playback. */
export function isTooSlow(session: SessionSummary): boolean {
  return session.transcodeSpeed !== undefined && session.transcodeSpeed < 1;
}

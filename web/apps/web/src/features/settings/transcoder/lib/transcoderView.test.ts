import { describe, expect, it } from "vitest";

import type { SessionSummary, TranscoderDevice, TranscoderStatus } from "../api/transcoderApi";
import {
  bitRateParts,
  deviceResult,
  hasUsableDevice,
  isSoftwareOnly,
  isTooSlow,
  speedValue,
} from "./transcoderView";

const device: TranscoderDevice = {
  api: "vaapi",
  encoder: "h264_vaapi",
  codec: "h264",
  usable: true,
  degraded: false,
};

const session: SessionSummary = {
  id: "5d4c3b2a-1f0e-4d9c-8b7a-6f5e4d3c2b1a",
  itemId: "8a9b0c1d-2e3f-4a5b-8c6d-7e8f9a0b1c2d",
  title: "Big Buck Bunny",
  state: "ready",
  mode: "transcode",
  hardware: false,
  clientName: "chromium-desktop",
  startedAt: "2026-11-11T19:30:00Z",
};

describe("deviceResult", () => {
  it.each([
    ["usable", { usable: true, degraded: false }, "usable"],
    ["not available", { usable: false, degraded: false }, "unavailable"],
    ["degraded", { usable: true, degraded: true }, "degraded"],
    ["degraded and untested", { usable: false, degraded: true }, "degraded"],
  ])("%s", (_name, flags, expected) => {
    expect(deviceResult({ ...device, ...flags })).toBe(expected);
  });
});

describe("software only", () => {
  const status = (devices: TranscoderDevice[]): TranscoderStatus => ({
    hwaccel: "auto",
    devices,
    throttleSupported: true,
  });

  it("is true without a usable encoder", () => {
    expect(isSoftwareOnly(status([]))).toBe(true);
    expect(isSoftwareOnly(status([{ ...device, usable: false }]))).toBe(true);
    expect(isSoftwareOnly(status([{ ...device, degraded: true }]))).toBe(true);
  });

  it("is false with one usable encoder", () => {
    expect(hasUsableDevice(status([{ ...device, usable: false }, device]))).toBe(true);
    expect(isSoftwareOnly(status([device]))).toBe(false);
  });
});

describe("bitRateParts", () => {
  it.each([
    [undefined, null],
    [8_000_000, { unit: "mbit", value: 8 }],
    [2_345_000, { unit: "mbit", value: 2.3 }],
    [640_400, { unit: "kbit", value: 640 }],
    [0, { unit: "kbit", value: 0 }],
  ])("%s", (bitsPerSecond, expected) => {
    expect(bitRateParts(bitsPerSecond)).toEqual(expected);
  });
});

describe("speed", () => {
  it("rounds to one decimal and keeps unknown as null", () => {
    expect(speedValue(2.44)).toBe(2.4);
    expect(speedValue(undefined)).toBeNull();
  });

  it("flags a session slower than real time", () => {
    expect(isTooSlow({ ...session, transcodeSpeed: 0.8 })).toBe(true);
    expect(isTooSlow({ ...session, transcodeSpeed: 1 })).toBe(false);
    expect(isTooSlow(session)).toBe(false);
  });
});

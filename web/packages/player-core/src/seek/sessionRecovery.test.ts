import { describe, expect, it } from "vitest";

import { shouldRecreateSession, type HlsErrorData, type PlaybackFailure } from "./sessionRecovery";

function hlsError(error: HlsErrorData): PlaybackFailure {
  return { source: "hls", error };
}

describe("shouldRecreateSession", () => {
  it.each<{ name: string; failure: PlaybackFailure; expected: boolean }>([
    {
      name: "404 on a session request",
      failure: { source: "http", status: 404, concernsSession: true },
      expected: true,
    },
    {
      name: "410 on a session request",
      failure: { source: "http", status: 410, concernsSession: true },
      expected: true,
    },
    {
      name: "404 on another request",
      failure: { source: "http", status: 404, concernsSession: false },
      expected: false,
    },
    {
      name: "503 on a session request",
      failure: { source: "http", status: 503, concernsSession: true },
      expected: false,
    },
    {
      name: "500 on a session request",
      failure: { source: "http", status: 500, concernsSession: true },
      expected: false,
    },
    {
      name: "a fatal segment load error",
      failure: hlsError({ type: "networkError", details: "fragLoadError", fatal: true }),
      expected: true,
    },
    {
      name: "a fatal segment load timeout",
      failure: hlsError({ type: "networkError", details: "fragLoadTimeOut", fatal: true }),
      expected: true,
    },
    {
      name: "a fatal segment error with a status",
      failure: hlsError({
        type: "networkError",
        details: "fragLoadError",
        fatal: true,
        response: { code: 500 },
      }),
      expected: true,
    },
    {
      name: "a non-fatal segment load error",
      failure: hlsError({ type: "networkError", details: "fragLoadError", fatal: false }),
      expected: false,
    },
    {
      name: "a fatal playlist error with 404",
      failure: hlsError({
        type: "networkError",
        details: "manifestLoadError",
        fatal: true,
        response: { code: 404 },
      }),
      expected: true,
    },
    {
      name: "a fatal playlist reload error with 410",
      failure: hlsError({
        type: "networkError",
        details: "levelLoadError",
        fatal: true,
        response: { code: 410 },
      }),
      expected: true,
    },
    {
      name: "a fatal playlist error with 503 (stream not ready)",
      failure: hlsError({
        type: "networkError",
        details: "manifestLoadError",
        fatal: true,
        response: { code: 503 },
      }),
      expected: false,
    },
    {
      name: "a fatal playlist timeout without a response",
      failure: hlsError({ type: "networkError", details: "manifestLoadTimeOut", fatal: true }),
      expected: false,
    },
    {
      name: "a fatal playlist error without a status code",
      failure: hlsError({
        type: "networkError",
        details: "manifestLoadError",
        fatal: true,
        response: {},
      }),
      expected: false,
    },
    {
      name: "a fatal media error",
      failure: hlsError({ type: "mediaError", details: "bufferStalledError", fatal: true }),
      expected: false,
    },
    {
      name: "a fatal media error on a segment",
      failure: hlsError({ type: "mediaError", details: "fragLoadError", fatal: true }),
      expected: false,
    },
  ])("is $expected for $name", ({ failure, expected }) => {
    expect(shouldRecreateSession(failure)).toBe(expected);
  });
});

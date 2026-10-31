import { describe, expect, it } from "vitest";

import { playbackNotice } from "./playbackStatusText";

describe("playbackNotice", () => {
  it.each([
    {
      view: { status: "loading", waitingForStream: false },
      expected: { kind: "loading", key: "status.loading" },
    },
    {
      view: { status: "loading", waitingForStream: true },
      expected: { kind: "loading", key: "status.waitingForStream" },
    },
    { view: { status: "playing", waitingForStream: false }, expected: { kind: "none" } },
    {
      view: { status: "error", waitingForStream: false },
      expected: { kind: "error", key: "errors.generic", retryable: true },
    },
  ] as const)("shows $expected.kind while $view.status", ({ view, expected }) => {
    expect(playbackNotice({ error: undefined, ...view })).toEqual(expected);
  });

  it.each([
    { code: "no_compatible_format", retryable: false },
    { code: "transcode_unavailable", retryable: false },
    { code: "session_limit_reached", retryable: true },
    { code: "stream_not_ready", retryable: true },
    { code: "generic", retryable: true },
  ] as const)("offers a retry for $code: $retryable", ({ code, retryable }) => {
    expect(playbackNotice({ status: "error", waitingForStream: false, error: { code } })).toEqual({
      kind: "error",
      key: `errors.${code}`,
      retryable,
    });
  });
});

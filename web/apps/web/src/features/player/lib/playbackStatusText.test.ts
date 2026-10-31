import { describe, expect, it } from "vitest";

import { playbackStatusKey } from "./playbackStatusText";

describe("playbackStatusKey", () => {
  it.each([
    { view: { status: "loading", waitingForStream: false }, expected: "status.loading" },
    { view: { status: "loading", waitingForStream: true }, expected: "status.waitingForStream" },
    { view: { status: "playing", waitingForStream: false }, expected: "status.playing" },
    {
      view: { status: "error", waitingForStream: false, error: { code: "transcode_unavailable" } },
      expected: "errors.transcode_unavailable",
    },
    { view: { status: "error", waitingForStream: false }, expected: "errors.generic" },
  ] as const)("maps $view.status to $expected", ({ view, expected }) => {
    expect(playbackStatusKey({ error: undefined, ...view })).toBe(expected);
  });
});

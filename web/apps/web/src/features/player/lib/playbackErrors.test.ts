import { describe, expect, it } from "vitest";

import { PlaybackRequestError, toPlaybackError } from "./playbackErrors";

function requestError(status: number, code: string, retryAfterSeconds?: number) {
  return new PlaybackRequestError({ status, code, retryAfterSeconds });
}

describe("toPlaybackError", () => {
  it.each(["no_compatible_format", "transcode_unavailable", "stream_not_ready"])(
    "keeps the code %s",
    (code) => {
      expect(toPlaybackError(requestError(422, code))).toEqual({ code });
    },
  );

  it("keeps the Retry-After delay of session_limit_reached", () => {
    expect(toPlaybackError(requestError(503, "session_limit_reached", 5))).toEqual({
      code: "session_limit_reached",
      retryAfterSeconds: 5,
    });
  });

  it.each([
    { name: "an unknown server code", error: requestError(422, "unsupported") },
    { name: "a network failure", error: requestError(0, "network_error") },
    { name: "another error", error: new Error("boom") },
    { name: "a thrown string", error: "boom" },
  ])("treats $name as generic", ({ error }) => {
    expect(toPlaybackError(error)).toEqual({ code: "generic" });
  });
});

describe("PlaybackRequestError", () => {
  it("carries status, code and Retry-After in a readable message", () => {
    const error = requestError(503, "session_limit_reached", 7);

    expect(error).toBeInstanceOf(Error);
    expect(error.name).toBe("PlaybackRequestError");
    expect(error.message).toBe("Playback request failed: session_limit_reached (HTTP 503)");
    expect(error).toMatchObject({ status: 503, retryAfterSeconds: 7 });
  });
});

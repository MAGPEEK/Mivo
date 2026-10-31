import { describe, expect, it, vi } from "vitest";

import { setVolume, toggleMute, togglePlayback, type ControllableMedia } from "./mediaActions";

interface FakeMedia extends ControllableMedia {
  readonly playCalls: () => number;
  readonly pauseCalls: () => number;
}

function fakeMedia(
  overrides: Partial<Pick<ControllableMedia, "paused" | "volume">> = {},
  playResult: () => Promise<void> = () => Promise.resolve(),
): FakeMedia {
  const play = vi.fn(playResult);
  const pause = vi.fn();
  return {
    paused: true,
    muted: false,
    volume: 1,
    ...overrides,
    play,
    pause,
    playCalls: () => play.mock.calls.length,
    pauseCalls: () => pause.mock.calls.length,
  };
}

describe("togglePlayback", () => {
  it("plays a paused element", async () => {
    const media = fakeMedia();

    togglePlayback(media);

    await vi.waitFor(() => {
      expect(media.playCalls()).toBe(1);
    });
    expect(media.pauseCalls()).toBe(0);
  });

  it("pauses a playing element", () => {
    const media = fakeMedia({ paused: false });

    togglePlayback(media);

    expect(media.pauseCalls()).toBe(1);
    expect(media.playCalls()).toBe(0);
  });

  it("stays quiet when the browser refuses to play", async () => {
    const media = fakeMedia({}, () => Promise.reject(new Error("NotAllowedError")));

    togglePlayback(media);

    await vi.waitFor(() => {
      expect(media.playCalls()).toBe(1);
    });
  });
});

describe("toggleMute", () => {
  it("flips muted and keeps the volume", () => {
    const media = fakeMedia({ volume: 0.4 });

    toggleMute(media);
    expect(media).toMatchObject({ muted: true, volume: 0.4 });
    toggleMute(media);
    expect(media).toMatchObject({ muted: false, volume: 0.4 });
  });
});

describe("setVolume", () => {
  it("mutes at zero and unmutes above", () => {
    const media = fakeMedia();

    setVolume(media, 0);
    expect(media).toMatchObject({ muted: true, volume: 0 });
    setVolume(media, 0.5);
    expect(media).toMatchObject({ muted: false, volume: 0.5 });
  });
});

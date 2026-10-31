import type { TimeRange } from "@mivo/player-core";
import { act, cleanup, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { controlsHideDelayMilliseconds } from "../hooks/useControlsVisibility";
import { stubFullscreen } from "../test/fakeFullscreen";
import { directPlaySession, playerItemId, remuxSession } from "../test/playerFixtures";
import { renderPlayer } from "../test/renderPlayer";

vi.mock("../hooks/usePlaybackSession");

const playing = { status: "playing", session: directPlaySession, position: 30 } as const;

function controls(): HTMLElement {
  return screen.getByRole("group", { name: "Player controls" });
}

function scrubber(): HTMLElement {
  return screen.getByRole("slider", { name: "Position" });
}

/** Makes the element report a play state and fires the matching event, like a browser. */
function setPaused(video: HTMLVideoElement, paused: boolean): void {
  Object.defineProperty(video, "paused", { configurable: true, value: paused });
  fireEvent(video, new Event(paused ? "pause" : "play"));
}

function setSeekable(video: HTMLVideoElement, ranges: readonly TimeRange[]): void {
  Object.defineProperty(video, "seekable", {
    configurable: true,
    value: {
      length: ranges.length,
      start: (index: number) => ranges[index]?.start ?? Number.NaN,
      end: (index: number) => ranges[index]?.end ?? Number.NaN,
    },
  });
  fireEvent(video, new Event("progress"));
}

function pressKey(key: string, target: Element = document.body, init: KeyboardEventInit = {}) {
  return fireEvent.keyDown(target, { key, ...init });
}

let play: ReturnType<typeof vi.spyOn>;
let pause: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  // jsdom implements none of them.
  play = vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue(undefined);
  pause = vi.spyOn(HTMLMediaElement.prototype, "pause").mockReturnValue(undefined);
  vi.spyOn(HTMLMediaElement.prototype, "load").mockReturnValue(undefined);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("PlayerPage control bar", () => {
  it("shows the title and labelled controls over a video without native controls", async () => {
    const { playback, video } = await renderPlayer(playing);

    expect(await screen.findByRole("heading", { level: 1, name: "Big Buck Bunny" })).toBeTruthy();
    expect(playback.attachVideo).toHaveBeenCalledWith(video);
    expect(video.hasAttribute("controls")).toBe(false);
    const bar = within(controls());
    for (const name of ["Play", "Mute", "Stream info"]) {
      expect(bar.getByRole("button", { name })).toBeTruthy();
    }
    expect(scrubber().getAttribute("aria-valuetext")).toBe("0:00:30 of 0:02:00");
    expect(bar.getByRole("slider", { name: "Volume" }).getAttribute("aria-valuetext")).toBe(
      "100 %",
    );
    expect(bar.getByText("0:30 / 2:00")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Back" })).toBeTruthy();
  });

  it("disables the scrubber and shows the bare position until the duration is known", async () => {
    await renderPlayer({ position: 0 });

    expect(scrubber()).toHaveProperty("disabled", true);
    expect(scrubber().getAttribute("aria-valuetext")).toBe("0:00:00");
    expect(within(controls()).getByText("0:00")).toBeTruthy();
    // Nothing plays yet, so the controls stay.
    expect(controls().className).toContain("opacity-100");
  });

  it("seeks on a click on the track and ±10 s with the arrow keys", async () => {
    const { playback } = await renderPlayer(playing);

    fireEvent.change(scrubber(), { target: { value: "50" } });
    pressKey("ArrowRight", scrubber());
    pressKey("ArrowDown", scrubber());
    const homeHandledNatively = pressKey("Home", scrubber());

    expect(playback.seek.mock.calls).toEqual([[50], [40], [20]]);
    expect(homeHandledNatively).toBe(true);
  });

  it("seeks once when a drag ends, showing the dragged position meanwhile", async () => {
    const { playback } = await renderPlayer(playing);

    fireEvent.pointerDown(scrubber());
    fireEvent.change(scrubber(), { target: { value: "60" } });
    fireEvent.change(scrubber(), { target: { value: "70" } });
    expect(scrubber().getAttribute("aria-valuetext")).toBe("0:01:10 of 0:02:00");
    expect(playback.seek).not.toHaveBeenCalled();
    fireEvent.pointerUp(window);
    fireEvent.pointerUp(window);

    expect(playback.seek.mock.calls).toEqual([[70]]);
    expect(scrubber().getAttribute("aria-valuetext")).toBe("0:00:30 of 0:02:00");
  });

  it("does not seek when a drag ends without moving or is cancelled", async () => {
    const { playback } = await renderPlayer(playing);

    fireEvent.pointerDown(scrubber());
    fireEvent.pointerUp(window);
    fireEvent.pointerDown(scrubber());
    fireEvent.change(scrubber(), { target: { value: "70" } });
    fireEvent.pointerCancel(window);

    expect(playback.seek).not.toHaveBeenCalled();
    expect(scrubber().getAttribute("aria-valuetext")).toBe("0:00:30 of 0:02:00");
  });

  it("stops listening for the end of a drag when the page goes away", async () => {
    await renderPlayer(playing);
    const removeListener = vi.spyOn(window, "removeEventListener");
    fireEvent.pointerDown(scrubber());

    cleanup();

    expect(removeListener).toHaveBeenCalledWith("pointerup", expect.any(Function));
    expect(removeListener).toHaveBeenCalledWith("pointercancel", expect.any(Function));
  });

  it("keeps the controls while keyboard focus is on one of them", async () => {
    const { video } = await renderPlayer(playing);
    vi.useFakeTimers();
    try {
      setPaused(video, false);
      const muteButton = within(controls()).getByRole("button", { name: "Mute" });
      // Focus by keyboard, so it is :focus-visible (a pointer before would make it a click).
      pressKey("Tab");
      act(() => {
        muteButton.focus();
      });
      expect(muteButton.matches(":focus-visible")).toBe(true);
      act(() => {
        vi.advanceTimersByTime(controlsHideDelayMilliseconds * 2);
      });
      expect(controls().className).toContain("opacity-100");

      act(() => {
        muteButton.blur();
      });
      act(() => {
        vi.advanceTimersByTime(controlsHideDelayMilliseconds);
      });

      expect(controls().className).toContain("opacity-0");
    } finally {
      vi.useRealTimers();
    }
  });

  it("does not count focus outside the controls", async () => {
    const { video } = await renderPlayer(playing);
    const elsewhere = document.createElement("button");
    document.body.append(elsewhere);
    vi.useFakeTimers();
    try {
      setPaused(video, false);
      pressKey("Tab");
      act(() => {
        within(controls()).getByRole("button", { name: "Mute" }).focus();
      });
      act(() => {
        elsewhere.focus();
      });
      act(() => {
        vi.advanceTimersByTime(controlsHideDelayMilliseconds);
      });

      expect(controls().className).toContain("opacity-0");
    } finally {
      vi.useRealTimers();
      elsewhere.remove();
    }
  });

  it("hides the controls and the pointer after three seconds of playback", async () => {
    const { video } = await renderPlayer(playing);
    vi.useFakeTimers();
    try {
      setPaused(video, false);
      act(() => {
        vi.advanceTimersByTime(controlsHideDelayMilliseconds);
      });
      expect(controls().className).toContain("opacity-0");
      expect(video.parentElement?.className).toContain("cursor-none");

      fireEvent.pointerMove(window);

      expect(controls().className).toContain("opacity-100");
      expect(video.parentElement?.className).not.toContain("cursor-none");
    } finally {
      vi.useRealTimers();
    }
  });

  it("shades the produced part of an HLS stream on the file timeline", async () => {
    const { video } = await renderPlayer({ ...playing, session: remuxSession });

    setSeekable(video, [{ start: 0, end: 30 }]);

    const [range] = screen.getAllByTestId("produced-range");
    expect(range?.style.left).toBe("25%");
    expect(range?.style.width).toBe("25%");
  });

  it("plays and pauses the video with the button", async () => {
    const { video } = await renderPlayer(playing);

    fireEvent.click(within(controls()).getByRole("button", { name: "Play" }));
    await vi.waitFor(() => {
      expect(play).toHaveBeenCalledOnce();
    });
    setPaused(video, false);
    fireEvent.click(within(controls()).getByRole("button", { name: "Pause" }));

    expect(pause).toHaveBeenCalledOnce();
  });

  it("mutes, unmutes and sets the volume", async () => {
    const { video } = await renderPlayer(playing);
    const bar = within(controls());

    fireEvent.click(bar.getByRole("button", { name: "Mute" }));
    fireEvent(video, new Event("volumechange"));
    expect(video.muted).toBe(true);
    expect(bar.getByRole("slider", { name: "Volume" }).getAttribute("aria-valuetext")).toBe("0 %");
    fireEvent.click(bar.getByRole("button", { name: "Unmute" }));
    fireEvent.change(bar.getByRole("slider", { name: "Volume" }), { target: { value: "0.5" } });
    fireEvent(video, new Event("volumechange"));

    expect(video).toMatchObject({ muted: false, volume: 0.5 });
    expect(bar.getByRole("slider", { name: "Volume" }).getAttribute("aria-valuetext")).toBe("50 %");
  });
});

describe("PlayerPage keyboard", () => {
  it("plays with Space and K and keeps the page from scrolling", async () => {
    await renderPlayer(playing);

    const notPrevented = pressKey(" ");
    pressKey("K");

    expect(notPrevented).toBe(false);
    await vi.waitFor(() => {
      expect(play).toHaveBeenCalledTimes(2);
    });
  });

  it("moves ±10 s with the arrow keys, never before the start", async () => {
    const { playback } = await renderPlayer(playing);

    pressKey("ArrowRight");
    pressKey("ArrowLeft");
    act(() => {
      playback.show({ position: 4 });
    });
    pressKey("ArrowLeft");

    expect(playback.seek.mock.calls).toEqual([[40], [20], [0]]);
  });

  it("mutes with M", async () => {
    const { video } = await renderPlayer(playing);

    pressKey("m");

    expect(video.muted).toBe(true);
  });

  it("leaves Space on a focused button and shortcuts with modifiers alone", async () => {
    const { video } = await renderPlayer(playing);
    const muteButton = within(controls()).getByRole("button", { name: "Mute" });

    pressKey(" ", muteButton);
    pressKey("m", document.body, { ctrlKey: true });

    expect(play).not.toHaveBeenCalled();
    expect(video.muted).toBe(false);
  });

  it("plays with K although a button has focus", async () => {
    await renderPlayer(playing);
    const muteButton = within(controls()).getByRole("button", { name: "Mute" });

    pressKey("k", muteButton);

    await vi.waitFor(() => {
      expect(play).toHaveBeenCalledOnce();
    });
  });

  it("does not toggle again while a key is held", async () => {
    const { video } = await renderPlayer(playing);

    pressKey("m");
    pressKey("m", document.body, { repeat: true });

    expect(video.muted).toBe(true);
  });
});

describe("PlayerPage play intent before a stream plays", () => {
  it("shows and sets the engine's intent instead of the empty element's state", async () => {
    const { playback } = await renderPlayer({ status: "loading" });
    const bar = within(controls());

    fireEvent.click(bar.getByRole("button", { name: "Pause" }));
    expect(playback.setAutoplay).toHaveBeenLastCalledWith(false);
    pressKey(" ");

    expect(playback.setAutoplay).toHaveBeenLastCalledWith(true);
    expect(bar.getByRole("button", { name: "Pause" })).toBeTruthy();
    expect(play).not.toHaveBeenCalled();
    expect(pause).not.toHaveBeenCalled();
  });
});

describe("PlayerPage full screen", () => {
  it("offers no full-screen button where the browser cannot go full screen", async () => {
    await renderPlayer(playing);

    expect(screen.queryByRole("button", { name: "Full screen" })).toBeNull();
  });

  it("shows the player with its controls full screen and leaves with F or Escape", async () => {
    const fullscreen = stubFullscreen();
    const { video } = await renderPlayer(playing);

    fireEvent.click(within(controls()).getByRole("button", { name: "Full screen" }));
    expect(fullscreen.requests).toEqual([video.parentElement]);
    expect(await screen.findByRole("button", { name: "Exit full screen" })).toBeTruthy();
    pressKey("f");
    await screen.findByRole("button", { name: "Full screen" });
    pressKey("Escape");
    expect(fullscreen.exits()).toBe(1);
    pressKey("F");
    await screen.findByRole("button", { name: "Exit full screen" });
    pressKey("Escape");

    expect(fullscreen.exits()).toBe(2);
    expect(await screen.findByRole("button", { name: "Full screen" })).toBeTruthy();
  });

  it("stays as it is when the browser refuses", async () => {
    const fullscreen = stubFullscreen({ refuseRequest: true });
    await renderPlayer(playing);

    pressKey("f");
    await Promise.resolve();

    expect(fullscreen.requests).toHaveLength(1);
    expect(screen.getByRole("button", { name: "Full screen" })).toBeTruthy();
  });

  it("stays full screen when the browser refuses to leave", async () => {
    const fullscreen = stubFullscreen({ refuseExit: true });
    await renderPlayer(playing);
    pressKey("f");
    await screen.findByRole("button", { name: "Exit full screen" });

    pressKey("f");
    await Promise.resolve();

    expect(fullscreen.exits()).toBe(1);
    expect(screen.getByRole("button", { name: "Exit full screen" })).toBeTruthy();
  });
});

describe("PlayerPage back button", () => {
  it("opens the item page when the player was opened directly", async () => {
    const { router } = await renderPlayer(playing);

    fireEvent.click(screen.getByRole("button", { name: "Back" }));

    await vi.waitFor(() => {
      expect(router.state.location.pathname).toBe(`/items/${playerItemId}`);
    });
  });

  it("goes back in the history when the app opened the player", async () => {
    const { router } = await renderPlayer(playing);
    await act(() =>
      router.navigate({
        to: "/items/$itemId/play",
        params: { itemId: playerItemId },
        search: { t: 60 },
      }),
    );

    fireEvent.click(screen.getByRole("button", { name: "Back" }));

    await vi.waitFor(() => {
      expect(router.state.location.search).toEqual({});
    });
    expect(router.state.location.pathname).toBe(`/items/${playerItemId}/play`);
  });
});

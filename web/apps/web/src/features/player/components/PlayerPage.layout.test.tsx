import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { playerMinimiseEvent, type PlayerMinimiseDetail } from "../lib/playerEvents";
import { volumeStorageKey } from "../lib/volumePreference";
import { directPlaySession, playerItem, playerItemId } from "../test/playerFixtures";
import { renderPlayer } from "../test/renderPlayer";

vi.mock("../hooks/usePlaybackSession");

const playing = { status: "playing", session: directPlaySession, position: 30 } as const;

function controls(): HTMLElement {
  return screen.getByRole("group", { name: "Player controls" });
}

function button(name: string): HTMLButtonElement {
  return within(controls()).getByRole<HTMLButtonElement>("button", { name });
}

beforeEach(() => {
  vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue(undefined);
  vi.spyOn(HTMLMediaElement.prototype, "pause").mockReturnValue(undefined);
  vi.spyOn(HTMLMediaElement.prototype, "load").mockReturnValue(undefined);
});

afterEach(() => {
  cleanup();
  window.localStorage.clear();
  vi.restoreAllMocks();
});

describe("PlayerPage title block", () => {
  it("links the title to the item and shows the year", async () => {
    await renderPlayer(playing, ["en"], { item: playerItem({ year: 2008 }) });

    const link = await screen.findByRole("link", { name: "Big Buck Bunny" });
    expect(link.getAttribute("href")).toBe(`/items/${playerItemId}`);
    expect(within(controls()).getByText("2008")).toBeTruthy();
  });

  it("links an episode to its show and shows the episode line", async () => {
    const item = playerItem({
      kind: "episode",
      title: "Pilot",
      number: 3,
      ancestors: [
        { id: "show-1", kind: "show", title: "The Show" },
        { id: "season-1", kind: "season", title: "Season 1", number: 1 },
      ],
    });
    await renderPlayer(playing, ["en"], { item });

    const heading = await screen.findByRole("heading", { level: 1, name: "The Show" });
    expect(within(heading).getByRole("link").getAttribute("href")).toBe("/items/show-1");
    expect(within(controls()).getByText("S1 · E3 · Pilot")).toBeTruthy();
  });
});

describe("PlayerPage transport", () => {
  it("marks previous and next unavailable without a queue but keeps them focusable", async () => {
    const { router } = await renderPlayer(playing);
    const next = button("Next");

    next.focus();
    fireEvent.click(next);

    expect(button("Previous").getAttribute("aria-disabled")).toBe("true");
    expect(next.getAttribute("aria-disabled")).toBe("true");
    expect(document.activeElement).toBe(next);
    expect(router.state.location.pathname).toBe(`/items/${playerItemId}/play`);
  });

  it("jumps 10 seconds back and 30 seconds forward within the file", async () => {
    const { playback } = await renderPlayer({ ...playing, position: 5 });

    fireEvent.click(button("Back 10 seconds"));
    expect(playback.seek).toHaveBeenLastCalledWith(0);
    fireEvent.click(button("Forward 30 seconds"));
    expect(playback.seek).toHaveBeenLastCalledWith(35);
  });

  it("does not jump past the end of the file", async () => {
    const { playback } = await renderPlayer({ ...playing, position: 100 });

    fireEvent.click(button("Forward 30 seconds"));

    expect(playback.seek).toHaveBeenLastCalledWith(120);
  });

  it("stops by returning to the item page", async () => {
    const { router } = await renderPlayer(playing);

    fireEvent.click(button("Stop"));

    await vi.waitFor(() => {
      expect(router.state.location.pathname).toBe(`/items/${playerItemId}`);
    });
  });

  it("dispatches the minimise event with the item and position", async () => {
    const received: PlayerMinimiseDetail[] = [];
    const listener = (event: Event) => {
      if (event instanceof CustomEvent) {
        received.push(event.detail as PlayerMinimiseDetail);
      }
    };
    window.addEventListener(playerMinimiseEvent, listener);
    await renderPlayer(playing);

    fireEvent.click(screen.getByRole("button", { name: "Minimise" }));

    window.removeEventListener(playerMinimiseEvent, listener);
    expect(received).toEqual([{ itemId: playerItemId, position: 30 }]);
  });
});

describe("PlayerPage repeat and shuffle", () => {
  it("cycles repeat off, all, one and announces the mode", async () => {
    await renderPlayer(playing);
    const repeat = button("Repeat");
    expect(repeat.getAttribute("aria-pressed")).toBe("false");

    fireEvent.click(repeat);
    expect(button("Repeat").getAttribute("aria-pressed")).toBe("true");
    expect(within(controls()).getByText("Repeat all")).toBeTruthy();
    fireEvent.click(button("Repeat"));
    expect(within(controls()).getByText("Repeat one")).toBeTruthy();
    fireEvent.click(button("Repeat"));
    expect(button("Repeat").getAttribute("aria-pressed")).toBe("false");
    expect(within(controls()).getByText("Repeat off")).toBeTruthy();
  });

  it("toggles shuffle and announces it", async () => {
    await renderPlayer(playing);

    fireEvent.click(button("Shuffle"));
    expect(button("Shuffle").getAttribute("aria-pressed")).toBe("true");
    expect(within(controls()).getByText("Shuffle on")).toBeTruthy();
    fireEvent.click(button("Shuffle"));
    expect(button("Shuffle").getAttribute("aria-pressed")).toBe("false");
    expect(within(controls()).getByText("Shuffle off")).toBeTruthy();
  });

  it("names the controls in German", async () => {
    await renderPlayer(playing, ["de"]);

    const bar = within(screen.getByRole("group", { name: "Wiedergabesteuerung" }));
    for (const name of ["Vorheriges", "Nächstes", "Stopp", "Wiederholen", "Zufallswiedergabe"]) {
      expect(bar.getByRole("button", { name })).toBeTruthy();
    }
    expect(screen.getByRole("button", { name: "Minimieren" })).toBeTruthy();
  });
});

describe("PlayerPage volume", () => {
  it("keeps the volume per device", async () => {
    const { video } = await renderPlayer(playing);

    fireEvent.change(within(controls()).getByRole("slider", { name: "Volume" }), {
      target: { value: "0.4" },
    });
    fireEvent(video, new Event("volumechange"));

    expect(JSON.parse(window.localStorage.getItem(volumeStorageKey) ?? "null")).toEqual({
      volume: 0.4,
      muted: false,
    });
  });

  it("never saves volume 0, so the next start is not silent", async () => {
    const { video } = await renderPlayer(playing);
    const slider = within(controls()).getByRole("slider", { name: "Volume" });

    fireEvent.change(slider, { target: { value: "0.6" } });
    fireEvent(video, new Event("volumechange"));
    fireEvent.change(slider, { target: { value: "0" } });
    fireEvent(video, new Event("volumechange"));

    expect(JSON.parse(window.localStorage.getItem(volumeStorageKey) ?? "null")).toEqual({
      volume: 0.6,
      muted: true,
    });
  });

  it("starts with the saved volume", async () => {
    window.localStorage.setItem(volumeStorageKey, JSON.stringify({ volume: 0.25, muted: true }));

    const { video } = await renderPlayer(playing);

    expect(video.volume).toBe(0.25);
    expect(video.muted).toBe(true);
  });
});

import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import { stubMediaQueries } from "../../../test/mediaQuery";
import { autoPlayStorageKey } from "../lib/autoPlayPreference";
import { speedStorageKey } from "../lib/playbackSpeed";
import { playerTracks } from "../lib/playerTracks";
import { qualityStorageKey } from "../lib/qualityPreference";
import { subtitleSizeStorageKey } from "../lib/subtitleSizePreference";
import {
  englishSubtitleId,
  imageSubtitleId,
  multiTrackItem,
  multiTrackSession,
  mseProfile,
  playerFileId,
} from "../test/playerFixtures";
import { renderPlayer, type RenderedPlayer } from "../test/renderPlayer";

vi.mock("../hooks/usePlaybackSession");

const playing = {
  status: "playing",
  session: multiTrackSession,
  position: 30,
  tracks: playerTracks(multiTrackItem(), playerFileId),
  profile: mseProfile,
} as const;

beforeEach(() => {
  vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue(undefined);
  vi.spyOn(HTMLMediaElement.prototype, "pause").mockReturnValue(undefined);
  vi.spyOn(HTMLMediaElement.prototype, "load").mockReturnValue(undefined);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

async function openPanel(
  languages: readonly string[] = ["en"],
  snapshot: Parameters<typeof renderPlayer>[0] = playing,
): Promise<RenderedPlayer> {
  const rendered = await renderPlayer(snapshot, languages);
  const name = languages[0] === "de" ? "Wiedergabeoptionen" : "Playback options";
  click(screen.getByRole("button", { name }));
  await screen.findByRole("dialog");
  return rendered;
}

function click(element: Element): void {
  fireEvent.click(element);
}

/** A key press on the focused element, which bubbles like the browser's. */
function press(key: string, init: KeyboardEventInit = {}): void {
  fireEvent.keyDown(document.activeElement ?? document.body, { key, ...init });
}

function menu(name: string): HTMLElement {
  return screen.getByRole("menu", { name });
}

function menuItem(menuName: string, index: number): HTMLElement {
  const item = within(menu(menuName)).getAllByRole("menuitemradio")[index];
  if (item === undefined) {
    throw new Error(`${menuName} has no item ${String(index)}`);
  }
  return item;
}

function items(name: string): string[] {
  return within(menu(name))
    .getAllByRole("menuitemradio")
    .map((item) => item.textContent);
}

function checked(name: string): string[] {
  return within(menu(name))
    .getAllByRole("menuitemradio", { checked: true })
    .map((item) => item.textContent);
}

describe("playback options panel", () => {
  it("opens from the control bar as a labelled dialog with focus on the playing audio track", async () => {
    await openPanel();

    const dialog = screen.getByRole("dialog", { name: "Playback options" });
    expect(dialog.getAttribute("aria-modal")).toBe("true");
    expect(
      screen.getByRole("button", { name: "Playback options" }).getAttribute("aria-expanded"),
    ).toBe("true");
    expect(document.activeElement).toBe(
      within(menu("Audio")).getByRole("menuitemradio", { checked: true }),
    );
  });

  it("closes on Escape and returns focus to the options button", async () => {
    await openPanel();

    press("Escape");

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Playback options" }));
  });

  it("closes with the close button and with a press on the video", async () => {
    await openPanel();
    click(screen.getByRole("button", { name: "Close playback options" }));
    expect(screen.queryByRole("dialog")).toBeNull();

    click(screen.getByRole("button", { name: "Playback options" }));
    fireEvent.mouseDown(screen.getByTestId("options-scrim"));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("wraps Tab at both ends of the dialog", async () => {
    await openPanel();
    const dialog = screen.getByRole("dialog");
    const focusable = [
      ...dialog.querySelectorAll<HTMLElement>("button:not([tabindex='-1']), input"),
    ];
    const first = focusable[0];
    const last = focusable.at(-1);

    last?.focus();
    press("Tab");
    expect(document.activeElement).toBe(first);

    press("Tab", { shiftKey: true });
    expect(document.activeElement).toBe(last);
  });

  it("keeps the player's shortcuts out of the dialog", async () => {
    const { playback } = await openPanel();

    press("ArrowRight");
    press("k");

    expect(playback.seek).not.toHaveBeenCalled();
    expect(playback.setAutoplay).not.toHaveBeenCalled();
  });

  it("has no serious accessibility violations", async () => {
    await openPanel();

    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });
});

describe("audio section", () => {
  it("lists the tracks in file order with language, title, codec and channels", async () => {
    await openPanel();

    expect(items("Audio")).toEqual([
      "GermanDeutsch 5.1 · E-AC-3 · 5.1is converted to AAC",
      "EnglishAAC · 2.0",
      "EnglishDirector's commentary · AAC · 2.0",
    ]);
    expect(checked("Audio")).toEqual(["GermanDeutsch 5.1 · E-AC-3 · 5.1is converted to AAC"]);
    expect(screen.getByText("Switching restarts the stream briefly.")).toBeTruthy();
  });

  it("restarts with the chosen track and shows the spinner text while it switches", async () => {
    const { playback } = await openPanel();

    click(menuItem("Audio", 1));

    expect(playback.applySwitch).toHaveBeenCalledWith({
      kind: "restart",
      selection: { audioStreamIndex: 2 },
    });
    act(() => {
      playback.show({ status: "loading", switching: true });
    });
    expect(await screen.findByText(/Switching … playback continues/)).toBeTruthy();
  });

  it("does nothing for the track that already plays", async () => {
    const { playback } = await openPanel();

    click(menuItem("Audio", 0));

    expect(playback.applySwitch).toHaveBeenCalledWith({ kind: "none" });
  });

  it("moves focus with the arrow keys without choosing", async () => {
    const { playback } = await openPanel();

    press("ArrowDown");

    expect(document.activeElement).toBe(within(menu("Audio")).getAllByRole("menuitemradio")[1]);
    press("ArrowUp");
    expect(document.activeElement).toBe(within(menu("Audio")).getAllByRole("menuitemradio")[0]);
    press("End");
    expect(document.activeElement).toBe(within(menu("Audio")).getAllByRole("menuitemradio")[2]);
    expect(playback.applySwitch).not.toHaveBeenCalled();
  });

  it("says so when the file has a single audio track", async () => {
    const tracks = playerTracks(multiTrackItem(), playerFileId);
    await openPanel(["en"], { ...playing, tracks: { ...tracks, audio: tracks.audio.slice(0, 1) } });

    expect(screen.getByText("This file has only one audio track.")).toBeTruthy();
  });
});

describe("subtitle section", () => {
  it("lists Off and the tracks with their badges and what they cost", async () => {
    await openPanel();

    expect(items("Subtitles")).toEqual([
      "Off",
      "GermanDeutsch (Forced)ForcedSRT",
      "EnglishSDHSRT",
      "EnglishPGSis burned in – the video is converted",
      "FrenchExternalASSwithout styling",
    ]);
    expect(checked("Subtitles")).toEqual(["Off"]);
    expect(screen.getByText("Text subtitles switch instantly.")).toBeTruthy();
  });

  it("switches a text track without a restart", async () => {
    const { playback } = await openPanel();

    click(menuItem("Subtitles", 2));

    expect(playback.applySwitch).toHaveBeenCalledWith({
      kind: "sidecar",
      track: expect.objectContaining({ id: englishSubtitleId }) as unknown,
      selection: { subtitleTrackId: englishSubtitleId },
    });
    expect(checked("Subtitles")).toEqual(["EnglishSDHSRT"]);
  });

  it("restarts for an image track", async () => {
    const { playback } = await openPanel();

    click(menuItem("Subtitles", 3));

    expect(playback.applySwitch).toHaveBeenCalledWith({
      kind: "restart",
      selection: { subtitleTrackId: imageSubtitleId },
    });
  });

  it("notes on the row that the server could not burn in, with subtitles off", async () => {
    const { playback } = await openPanel();
    click(menuItem("Subtitles", 3));

    act(() => {
      playback.show({
        selection: { subtitleTrackId: "" },
        session: {
          ...multiTrackSession,
          warnings: [{ code: "subtitle_burn_in_unavailable", scope: "subtitle" }],
        },
      });
    });

    expect(checked("Subtitles")).toEqual(["Off"]);
    expect(
      within(menu("Subtitles")).getByText("Not possible right now: the server cannot burn in."),
    ).toBeTruthy();
  });

  it("says so when the file has no subtitles", async () => {
    const tracks = playerTracks(multiTrackItem(), playerFileId);
    await openPanel(["en"], { ...playing, tracks: { ...tracks, subtitles: [] } });

    expect(items("Subtitles")).toEqual(["Off"]);
    expect(screen.getByText("This file has no subtitles.")).toBeTruthy();
  });
});

describe("subtitle size section", () => {
  it("stores the size per device", async () => {
    await openPanel();
    const group = screen.getByRole("radiogroup", { name: "Subtitle size" });
    expect(within(group).getByRole("radio", { name: "Normal" }).getAttribute("aria-checked")).toBe(
      "true",
    );

    click(within(group).getByRole("radio", { name: "Large" }));

    expect(window.localStorage.getItem(subtitleSizeStorageKey)).toBe("large");
    expect(within(group).getByRole("radio", { name: "Large" }).getAttribute("aria-checked")).toBe(
      "true",
    );
  });

  it("starts with the stored size", async () => {
    window.localStorage.setItem(subtitleSizeStorageKey, "huge");
    await openPanel();

    const group = screen.getByRole("radiogroup", { name: "Subtitle size" });
    expect(within(group).getByRole("radio", { name: "Huge" }).getAttribute("aria-checked")).toBe(
      "true",
    );
  });

  it("is disabled with a hint while a subtitle is burned in", async () => {
    await openPanel(["en"], {
      ...playing,
      session: { ...multiTrackSession, subtitle: { mode: "burn_in", trackId: imageSubtitleId } },
    });

    const group = screen.getByRole("radiogroup", { name: "Subtitle size" });
    for (const radio of within(group).getAllByRole("radio")) {
      expect(radio.matches(":disabled")).toBe(true);
    }
    expect(screen.getByText("Burned-in subtitles cannot be adjusted.")).toBeTruthy();
  });
});

describe("quality section", () => {
  it('shows Original and Automatic, with the steps behind "Show all steps"', async () => {
    await openPanel();

    expect(items("Quality")).toEqual(["Original", "Automatic"]);
    expect(checked("Quality")).toEqual(["Original"]);

    click(screen.getByRole("button", { name: "Show all steps" }));

    // The 20 Mbit/s step lies above the 14.2 Mbit/s source and is not offered.
    expect(items("Quality")).toEqual([
      "Original",
      "Automatic",
      "1080p · 12 Mbit/s",
      "1080p · 8 Mbit/s",
      "720p · 4 Mbit/s",
      "480p · 2 Mbit/s",
      "480p · 1 Mbit/s",
    ]);
    expect(screen.getByRole("button", { name: "Show fewer" }).getAttribute("aria-expanded")).toBe(
      "true",
    );
  });

  it("limits the quality with a restart and remembers it for this device", async () => {
    const { playback } = await openPanel();
    click(screen.getByRole("button", { name: "Show all steps" }));

    click(within(menu("Quality")).getByRole("menuitemradio", { name: "720p · 4 Mbit/s" }));

    expect(playback.applySwitch).toHaveBeenCalledWith({
      kind: "restart",
      selection: { maxBitRate: 4_000_000 },
    });
    expect(playback.setAutomaticQuality).toHaveBeenLastCalledWith(false);
    expect(window.localStorage.getItem(qualityStorageKey)).toBe("4000000");
    expect(checked("Quality")).toEqual(["720p · 4 Mbit/s"]);
  });

  it("keeps a chosen step visible when the list is collapsed", async () => {
    window.localStorage.setItem(qualityStorageKey, "8000000");
    await openPanel();

    expect(items("Quality")).toEqual(["Original", "Automatic", "1080p · 8 Mbit/s"]);
  });

  it("turns Automatic on without restarting and says what it plays now", async () => {
    const { playback } = await openPanel();

    click(within(menu("Quality")).getByRole("menuitemradio", { name: "Automatic" }));

    expect(playback.setAutomaticQuality).toHaveBeenLastCalledWith(true);
    expect(playback.applySwitch).not.toHaveBeenCalled();
    expect(window.localStorage.getItem(qualityStorageKey)).toBe("automatic");
    expect(checked("Quality")).toEqual(["Automaticcurrently Original"]);
  });

  it("offers no Automatic without HLS through MSE", async () => {
    await openPanel(["en"], {
      ...playing,
      profile: { ...mseProfile, hls: { ...mseProfile.hls, delivery: "native" } },
    });

    expect(items("Quality")).toEqual(["Original"]);
  });

  it("goes back to the original for a stored step above the source", async () => {
    window.localStorage.setItem(qualityStorageKey, "20000000");
    await openPanel();

    expect(checked("Quality")).toEqual(["Original"]);
  });
});

describe("playback section", () => {
  beforeEach(() => {
    window.localStorage.removeItem(autoPlayStorageKey);
    window.localStorage.removeItem(speedStorageKey);
  });

  it("offers auto play, on by default, and the speed steps with 1× chosen", async () => {
    await openPanel();

    const autoPlay = screen.getByRole<HTMLInputElement>("checkbox", {
      name: "Play next automatically",
    });
    expect(autoPlay.checked).toBe(true);
    const speed = screen.getByRole<HTMLSelectElement>("combobox", { name: "Playback speed" });
    expect([...speed.options].map((option) => option.textContent)).toEqual([
      "0.5×",
      "0.75×",
      "1×",
      "1.25×",
      "1.5×",
      "1.75×",
      "2×",
    ]);
    expect(speed.value).toBe("1");
  });

  it("applies the chosen speed to the video with the pitch kept, and remembers it", async () => {
    await openPanel();

    fireEvent.change(screen.getByRole("combobox", { name: "Playback speed" }), {
      target: { value: "1.5" },
    });

    const video = document.querySelector("video");
    expect(video?.playbackRate).toBe(1.5);
    expect(video?.preservesPitch).toBe(true);
    expect(window.localStorage.getItem(speedStorageKey)).toBe("1.5");
  });

  it("starts with the saved speed and auto play choice", async () => {
    window.localStorage.setItem(speedStorageKey, "0.75");
    window.localStorage.setItem(autoPlayStorageKey, "false");
    await openPanel();

    expect(document.querySelector("video")?.playbackRate).toBe(0.75);
    expect(screen.getByRole<HTMLSelectElement>("combobox", { name: "Playback speed" }).value).toBe(
      "0.75",
    );
    expect(
      screen.getByRole<HTMLInputElement>("checkbox", { name: "Play next automatically" }).checked,
    ).toBe(false);
  });

  it("saves the auto play choice and keeps the keys inside the panel", async () => {
    const { playback } = await openPanel();
    const autoPlay = screen.getByRole("checkbox", { name: "Play next automatically" });

    click(autoPlay);
    autoPlay.focus();
    press("ArrowLeft");

    expect(window.localStorage.getItem(autoPlayStorageKey)).toBe("false");
    expect(playback.seek).not.toHaveBeenCalled();
  });

  it("is in German, too, and has no serious accessibility violations", async () => {
    await openPanel(["de"]);

    expect(screen.getByRole("checkbox", { name: "Nächste automatisch abspielen" })).toBeTruthy();
    expect(screen.getByRole("combobox", { name: "Geschwindigkeit" })).toBeTruthy();
    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });
});

describe("toasts", () => {
  it("tells that the server could not burn the subtitle in, once, with a close button", async () => {
    const session = {
      ...multiTrackSession,
      warnings: [{ code: "subtitle_burn_in_unavailable", scope: "subtitle" } as const],
    };
    const { playback } = await renderPlayer({ ...playing, session });

    expect(screen.queryByRole("alert")).toBeNull();
    act(() => {
      playback.show({ switchedSessionId: session.id });
    });

    expect((await screen.findByRole("alert")).textContent).toContain(
      "This subtitle has to be burned into the video.",
    );
    click(screen.getByRole("button", { name: "Close message" }));
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("tells that the quality cap converts the video", async () => {
    const session = {
      ...multiTrackSession,
      reasons: [{ code: "bitrate_cap", scope: "video" } as const],
    };
    const { playback } = await renderPlayer({ ...playing, session });

    act(() => {
      playback.show({ switchedSessionId: session.id });
    });

    expect(
      await screen.findByText("The quality limit is below the source – the video is converted."),
    ).toBeTruthy();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("says nothing for a session no switch created", async () => {
    const session = {
      ...multiTrackSession,
      warnings: [{ code: "subtitle_burn_in_unavailable", scope: "subtitle" } as const],
    };

    await renderPlayer({ ...playing, session });

    expect(screen.queryByRole("alert")).toBeNull();
  });
});

describe("narrow screens", () => {
  it("shows one section per tab, starting on Audio", async () => {
    stubMediaQueries(true);
    await openPanel();

    const tabs = screen.getByRole("tablist", { name: "Option groups" });
    expect(
      within(tabs)
        .getAllByRole("tab")
        .map((tab) => tab.textContent),
    ).toEqual(["Quality", "Audio", "Subtitles", "Size", "Playback"]);
    expect(screen.queryByRole("menu", { name: "Subtitles" })).toBeNull();

    click(within(tabs).getByRole("tab", { name: "Subtitles" }));
    await waitFor(() => {
      expect(screen.getByRole("menu", { name: "Subtitles" })).toBeTruthy();
    });
  });
});

describe("German", () => {
  it("shows the panel in German with the language names in German", async () => {
    await openPanel(["de"]);

    expect(screen.getByRole("dialog", { name: "Wiedergabeoptionen" })).toBeTruthy();
    expect(items("Audio")[0]).toBe("DeutschDeutsch 5.1 · E-AC-3 · 5.1wird nach AAC umgewandelt");
    expect(items("Qualität")).toEqual(["Original", "Automatisch"]);
    expect(items("Untertitel")[0]).toBe("Aus");
    expect(screen.getByRole("radiogroup", { name: "Untertitelgröße" })).toBeTruthy();
  });
});

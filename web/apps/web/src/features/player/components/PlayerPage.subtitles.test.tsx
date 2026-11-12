import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { playerTracks } from "../lib/playerTracks";
import { subtitleSizeStorageKey } from "../lib/subtitleSizePreference";
import {
  englishSubtitleId,
  imageSubtitleId,
  multiTrackItem,
  multiTrackSession,
  mseProfile,
  playerFileId,
} from "../test/playerFixtures";
import { renderPlayer } from "../test/renderPlayer";

vi.mock("../hooks/usePlaybackSession");

const englishVtt = `WEBVTT

00:00:04.000 --> 00:00:08.000
Do you hear that?
It is coming back.

00:00:20.000 --> 00:00:22.000
<b>Run.</b>
`;

const germanVtt = `WEBVTT

00:00:04.000 --> 00:00:08.000
Hörst du das?
`;

const playing = {
  status: "playing",
  session: { ...multiTrackSession, delivery: "hls", startPositionSeconds: 0 },
  position: 5,
  tracks: playerTracks(multiTrackItem(), playerFileId),
  profile: mseProfile,
} as const;

/** Answers the subtitle URLs of the fixture file; the body is looked up by track. */
function stubSubtitleFetch(): ReturnType<typeof vi.fn> {
  const fetchSubtitle = vi.fn((url: string) =>
    Promise.resolve(
      url.includes("s:5.vtt")
        ? new Response(englishVtt)
        : url.includes("s:4.vtt")
          ? new Response(germanVtt)
          : new Response(null, { status: 404 }),
    ),
  );
  vi.stubGlobal("fetch", fetchSubtitle);
  return fetchSubtitle;
}

/** Moves the element's clock and tells the player, like the browser does while playing. */
function playAt(video: HTMLVideoElement, seconds: number): void {
  Object.defineProperty(video, "currentTime", { value: seconds, configurable: true });
  act(() => {
    fireEvent(video, new Event("timeupdate"));
  });
}

beforeEach(() => {
  vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue(undefined);
  vi.spyOn(HTMLMediaElement.prototype, "pause").mockReturnValue(undefined);
  vi.spyOn(HTMLMediaElement.prototype, "load").mockReturnValue(undefined);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("PlayerPage subtitles", () => {
  it("draws the cue of the chosen text track while it is current", async () => {
    stubSubtitleFetch();
    const { video } = await renderPlayer({
      ...playing,
      selection: { subtitleTrackId: englishSubtitleId },
    });

    playAt(video, 5);

    const layer = await screen.findByTestId("subtitle-layer");
    expect(layer.textContent).toBe("Do you hear that?\nIt is coming back.");
    expect(layer.getAttribute("aria-hidden")).toBe("true");

    playAt(video, 12);
    expect(screen.queryByTestId("subtitle-layer")).toBeNull();
    playAt(video, 21);
    expect((await screen.findByTestId("subtitle-layer")).textContent).toBe("Run.");
  });

  it("matches the cues against the file position of an HLS stream that starts late", async () => {
    stubSubtitleFetch();
    const { video } = await renderPlayer({
      ...playing,
      session: { ...playing.session, startPositionSeconds: 2 },
      selection: { subtitleTrackId: englishSubtitleId },
    });

    // The stream began at 2 s of the file: its clock at 0 s is file position 2 s, before the cue.
    playAt(video, 0);
    expect(screen.queryByTestId("subtitle-layer")).toBeNull();
    playAt(video, 3);
    expect((await screen.findByTestId("subtitle-layer")).textContent).toContain(
      "Do you hear that?",
    );
  });

  it("shows another track as soon as the choice changes, without a new session", async () => {
    const fetchSubtitle = stubSubtitleFetch();
    const { video, playback } = await renderPlayer({
      ...playing,
      selection: { subtitleTrackId: englishSubtitleId },
    });
    playAt(video, 5);
    await screen.findByTestId("subtitle-layer");

    act(() => {
      playback.show({ selection: { subtitleTrackId: "s:4" } });
    });

    expect((await screen.findByTestId("subtitle-layer")).textContent).toBe("Hörst du das?");
    expect(fetchSubtitle).toHaveBeenCalledTimes(2);
  });

  it("draws nothing for subtitles that are off, burned in or failed to load", async () => {
    stubSubtitleFetch();
    const { video, playback } = await renderPlayer(playing);
    playAt(video, 5);
    expect(screen.queryByTestId("subtitle-layer")).toBeNull();

    act(() => {
      playback.show({
        selection: { subtitleTrackId: imageSubtitleId },
        session: { ...playing.session, subtitle: { mode: "burn_in", trackId: imageSubtitleId } },
      });
    });
    playAt(video, 6);
    expect(screen.queryByTestId("subtitle-layer")).toBeNull();

    act(() => {
      playback.show({ selection: { subtitleTrackId: "x:9f2c41" } });
    });
    playAt(video, 7);
    expect(screen.queryByTestId("subtitle-layer")).toBeNull();
  });

  it("applies the size stored for this device", async () => {
    stubSubtitleFetch();
    window.localStorage.setItem(subtitleSizeStorageKey, "huge");
    const { video } = await renderPlayer({
      ...playing,
      selection: { subtitleTrackId: englishSubtitleId },
    });

    playAt(video, 5);

    const text = (await screen.findByTestId("subtitle-layer")).querySelector("p");
    expect(text?.className).toContain("md:text-5xl");
  });

  it("changes the size at once when the panel chooses another", async () => {
    stubSubtitleFetch();
    const { video } = await renderPlayer({
      ...playing,
      selection: { subtitleTrackId: englishSubtitleId },
    });
    playAt(video, 5);
    const layer = await screen.findByTestId("subtitle-layer");
    expect(layer.querySelector("p")?.className).toContain("md:text-xl");

    fireEvent.click(screen.getByRole("button", { name: "Playback options" }));
    fireEvent.click(await screen.findByRole("radio", { name: "Small" }));

    expect(screen.getByTestId("subtitle-layer").querySelector("p")?.className).toContain(
      "md:text-base",
    );
  });
});

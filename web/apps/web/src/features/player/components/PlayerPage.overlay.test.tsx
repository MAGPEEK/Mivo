import type { PlaybackSession } from "@mivo/player-core";
import { act, cleanup, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import type { PlaybackErrorCode } from "../lib/playbackErrors";
import {
  directPlaySession,
  playerItemId,
  remuxSession,
  transcodeSession,
} from "../test/playerFixtures";
import { renderPlayer } from "../test/renderPlayer";

vi.mock("../hooks/usePlaybackSession");

function overlay(): HTMLElement {
  return screen.getByRole("region", { name: "Stream info" });
}

/** Label and value of every row of the overlay's description list. */
function overlayRows(): string[][] {
  return within(overlay())
    .getAllByRole("term")
    .map((term) => [term.textContent, term.nextElementSibling?.textContent ?? ""]);
}

function listItems(name: string): string[] {
  return within(within(overlay()).getByRole("list", { name }))
    .getAllByRole("listitem")
    .map((item) => item.textContent);
}

async function openOverlay(session: PlaybackSession | undefined, languages?: readonly string[]) {
  const rendered = await renderPlayer({ status: "playing", session, position: 30 }, languages);
  fireEvent.keyDown(document.body, { key: "i" });
  await screen.findByRole("region");
  return rendered;
}

beforeEach(() => {
  vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue(undefined);
  vi.spyOn(HTMLMediaElement.prototype, "pause").mockReturnValue(undefined);
  vi.spyOn(HTMLMediaElement.prototype, "load").mockReturnValue(undefined);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("PlayerPage stream-info overlay", () => {
  it("shows a direct-play session in green without reasons or encoder", async () => {
    await openOverlay(directPlaySession);

    const badge = within(overlay()).getByText("Direct Play");
    expect(badge.className).toContain("bg-direct-play");
    expect(overlayRows()).toEqual([
      ["Container", "MP4"],
      ["Video", "H.264 · Copied"],
      ["Audio", "AAC · Copied"],
      ["Source bit rate", "950 kbit/s"],
    ]);
    expect(within(overlay()).queryByRole("list")).toBeNull();
  });

  it("shows a remux as Direct Stream in blue with its reason", async () => {
    await openOverlay(remuxSession);

    expect(within(overlay()).getByText("Direct Stream").className).toContain("bg-direct-stream");
    expect(listItems("Why")).toEqual(["MKV not supported → remux to MP4"]);
    expect(overlayRows()[0]).toEqual(["Container", "MKV → MP4"]);
    expect(overlayRows().at(-1)).toEqual(["Source bit rate", "4.1 Mbit/s"]);
  });

  it("shows a transcode in amber with reasons, streams, encoder and warnings", async () => {
    await openOverlay(transcodeSession);

    expect(within(overlay()).getByText("Transcode").className).toContain("bg-transcode");
    expect(listItems("Why")).toEqual([
      "MKV not supported → remux to MP4",
      "Video HEVC not supported → H.264",
      "Audio AC-3 not supported → AAC",
    ]);
    expect(overlayRows()).toEqual([
      ["Container", "MKV → MP4"],
      ["Video", "HEVC → H.264 · 1080p · Transcoded"],
      ["Audio", "AC-3 → AAC · 2.0 · Transcoded"],
      ["Encoder", "Software (libx264)"],
      ["Source bit rate", "18.5 Mbit/s"],
    ]);
    expect(listItems("Warnings")).toEqual([
      "HDR is converted without tone mapping, so colours may look pale",
      "Video downscaled to 1080p",
      "Subtitles are not shown yet",
    ]);
  });

  it("speaks German", async () => {
    await openOverlay(transcodeSession, ["de"]);

    const region = screen.getByRole("region", { name: "Stream-Info" });
    expect(within(region).getByText("Audio AC-3 nicht unterstützt → AAC")).toBeTruthy();
    expect(within(region).getByText("18,5 Mbit/s")).toBeTruthy();
    expect(within(region).getByText("Software (libx264)")).toBeTruthy();
  });

  it("shows modes and codes of a newer server with generic texts", async () => {
    const futureMode = "future_mode" as string;
    const futureSession: PlaybackSession = {
      ...directPlaySession,
      mode: futureMode as PlaybackSession["mode"],
      reasons: [{ code: "future_reason", scope: "audio" }],
    };
    delete futureSession.sourceBitRate;
    await openOverlay(futureSession);

    expect(within(overlay()).getByText("Other mode").className).toContain("bg-text-muted");
    expect(listItems("Why")).toEqual(["Other reason (future_reason)"]);
    // An unknown source bit rate is left out instead of reading "0 kbit/s".
    expect(overlayRows().map(([label]) => label)).toEqual(["Container", "Video", "Audio"]);
  });

  it("says so before the first session", async () => {
    await openOverlay(undefined);

    expect(within(overlay()).getByText("No stream yet.")).toBeTruthy();
  });

  it("moves focus into the overlay and back to its toggle when it closes", async () => {
    await renderPlayer({ status: "playing", session: directPlaySession });
    const toggle = screen.getByRole("button", { name: "Stream info" });
    expect(toggle.getAttribute("aria-expanded")).toBe("false");

    fireEvent.click(toggle);
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    expect(document.activeElement).toBe(within(overlay()).getByRole("heading", { level: 2 }));
    fireEvent.keyDown(document.body, { key: "Escape" });

    expect(screen.queryByRole("region")).toBeNull();
    expect(document.activeElement).toBe(toggle);
    fireEvent.click(toggle);
    fireEvent.click(within(overlay()).getByRole("button", { name: "Close stream info" }));
    expect(screen.queryByRole("region")).toBeNull();
    expect(document.activeElement).toBe(toggle);
  });

  it("leaves focus where it is when it closes from outside", async () => {
    await openOverlay(directPlaySession);
    const playButton = screen.getByRole("button", { name: "Play" });
    playButton.focus();

    fireEvent.click(screen.getByRole("button", { name: "Stream info" }));

    expect(screen.queryByRole("region")).toBeNull();
    expect(document.activeElement).toBe(playButton);
  });

  it("has no serious accessibility violations with the overlay open", async () => {
    await openOverlay(transcodeSession);

    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });
});

describe("PlayerPage states", () => {
  it("shows a spinner while loading and says when the server prepares the stream", async () => {
    const { playback } = await renderPlayer();

    const status = screen.getByRole("status");
    expect(status.textContent).toBe("Loading the video…");
    expect(status.querySelector(".animate-spin")?.className).toContain(
      "motion-reduce:animate-none",
    );
    act(() => {
      playback.show({ waitingForStream: true });
    });
    expect(status.textContent).toBe("The server is preparing the stream…");
    act(() => {
      playback.show({ status: "playing", waitingForStream: false, session: directPlaySession });
    });
    expect(status.textContent).toBe("");
  });

  it.each([
    {
      code: "session_limit_reached",
      text: "Too many videos are playing on the server right now. Try again in a moment.",
    },
    { code: "stream_not_ready", text: "The server could not prepare the stream in time." },
    { code: "generic", text: "This video cannot be played." },
  ] as const)("explains $code and focuses its retry button", async ({ code, text }) => {
    const { playback } = await renderPlayer({ status: "error", error: { code } });

    expect(screen.getByRole("alert").textContent).toBe(text);
    const retryButton = screen.getByRole("button", { name: "Try again" });
    expect(document.activeElement).toBe(retryButton);
    fireEvent.click(retryButton);

    expect(playback.retry).toHaveBeenCalledOnce();
    // The engine will play once the retry has a stream, so the button offers "Pause".
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Pause" }));
  });

  it.each([
    {
      code: "no_compatible_format",
      text: "This browser cannot play this file, not even after converting it.",
    },
    {
      code: "transcode_unavailable",
      text: "This file needs converting, but the server can only Direct Play and Remux.",
    },
  ] satisfies { code: PlaybackErrorCode; text: string }[])(
    "explains $code without a pointless retry",
    async ({ code, text }) => {
      const { router } = await renderPlayer({ status: "error", error: { code } });
      // Opened from within the app: the button still goes to the item, not back in history.
      await act(() =>
        router.navigate({
          to: "/items/$itemId/play",
          params: { itemId: playerItemId },
          search: { t: 60 },
        }),
      );

      expect(screen.getByRole("alert").textContent).toBe(text);
      expect(screen.queryByRole("button", { name: "Try again" })).toBeNull();
      const backButton = screen.getByRole("button", { name: "Back to the details" });
      expect(document.activeElement).toBe(backButton);
      fireEvent.click(backButton);

      await vi.waitFor(() => {
        expect(router.state.location.pathname).toBe(`/items/${playerItemId}`);
      });
    },
  );

  it("explains errors in German", async () => {
    await renderPlayer({ status: "error", error: { code: "transcode_unavailable" } }, ["de"]);

    expect(screen.getByRole("alert").textContent).toBe(
      "Diese Datei müsste umgewandelt werden, aber der Server kann nur Direct Play und Remux.",
    );
    expect(screen.getByRole("button", { name: "Zurück zu den Details" })).toBeTruthy();
  });

  it("has no serious accessibility violations while loading or failed", async () => {
    const { playback } = await renderPlayer();
    expect(await blockingAxeViolations(document.body)).toEqual([]);

    act(() => {
      playback.show({ status: "error", error: { code: "session_limit_reached" } });
    });

    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });
});

// Drives the player against the real server: start a session from the item page, watch the
// video play, read the stream-info overlay and leave the player again.
import type { PlaybackSession, Problem } from "@mivo/api-client";
import { colors } from "@mivo/ui-tokens";
import { expect, type Locator, type Page, type Response } from "@playwright/test";

import type { CorpusClip } from "./corpus";
import { openItem } from "./library";

const sessionsPath = "/api/v1/playback/sessions";
const createdStatus = 201;
const notFoundStatus = 404;

/** Clips last 2 s: a video past this position has played, not just loaded its first frame. */
export const playedSeconds = 1;

/** Time the server gets to produce the first HLS segments of a remux or transcode. */
const playbackTimeoutMs = 20_000;

function isSessionCreation(response: Response): boolean {
  return (
    response.request().method() === "POST" && new URL(response.url()).pathname === sessionsPath
  );
}

/** Waits for the next session creation request of `page` and returns its response. */
export function nextSessionResponse(page: Page): Promise<Response> {
  return page.waitForResponse(isSessionCreation);
}

/**
 * Opens `clip` from the library grid, presses Play and returns the server's answer to the
 * session request: a session (201) or problem details.
 */
export async function pressPlay(page: Page, clip: CorpusClip): Promise<Response> {
  await openItem(page, clip);
  const response = nextSessionResponse(page);
  await page.getByRole("link", { name: "Play" }).click();
  return response;
}

/** Like {@link pressPlay}, but expects a session and returns it. */
export async function startPlayback(page: Page, clip: CorpusClip): Promise<PlaybackSession> {
  const response = await pressPlay(page, clip);
  expect(response.status(), await response.text()).toBe(createdStatus);
  return (await response.json()) as PlaybackSession;
}

/** The code of the problem details in `response`. */
export async function problemCode(response: Response): Promise<string | undefined> {
  return ((await response.json()) as Problem).code;
}

/** The player's video element. */
export function playerVideo(page: Page): Locator {
  return page.getByLabel("Video", { exact: true });
}

function currentTime(video: Locator): Promise<number> {
  return video.evaluate((element) => (element as HTMLVideoElement).currentTime);
}

/** Waits until the video's clock passes {@link playedSeconds}. */
export async function expectVideoPlays(page: Page): Promise<void> {
  const video = playerVideo(page);
  await expect
    .poll(() => currentTime(video), { timeout: playbackTimeoutMs })
    .toBeGreaterThan(playedSeconds);
}

/** The position slider's value, i.e. the file position in seconds. */
export async function filePosition(page: Page): Promise<number> {
  return Number(await page.getByRole("slider", { name: "Position" }).inputValue());
}

/** Playback modes by their overlay label and status colour (ADR 0011). */
export const modes = {
  directPlay: { label: "Direct Play", color: colors.directPlay },
  remux: { label: "Direct Stream", color: colors.directStream },
  transcode: { label: "Transcode", color: colors.transcode },
} as const;

/** A playback mode of {@link modes}. */
export type Mode = (typeof modes)[keyof typeof modes];

/** `#RRGGBB` as the `rgb(r, g, b)` that `getComputedStyle` reports. */
function cssRgb(hex: string): string {
  const channels = [1, 3, 5].map((start) => Number.parseInt(hex.slice(start, start + 2), 16));
  return `rgb(${channels.join(", ")})`;
}

/** Opens the stream-info overlay from the keyboard and returns it. */
export async function openStreamInfo(page: Page): Promise<Locator> {
  await page.keyboard.press("i");
  const overlay = page.getByRole("region", { name: "Stream info" });
  await expect(overlay).toBeVisible();
  return overlay;
}

/** Expects the overlay to name `mode` on a badge in the mode's status colour. */
export async function expectMode(overlay: Locator, mode: Mode): Promise<void> {
  const badge = overlay.getByText(mode.label, { exact: true });
  await expect(badge).toBeVisible();
  await expect(badge).toHaveCSS("background-color", cssRgb(mode.color));
}

/** Expects the overlay's "Why" list to give exactly one reason matching `pattern`. */
export async function expectReason(overlay: Locator, pattern: RegExp): Promise<void> {
  const reasons = overlay.getByRole("list", { name: "Why" }).getByRole("listitem");
  await expect(reasons.filter({ hasText: pattern })).toHaveCount(1);
}

/**
 * Expects the server to know no session `sessionId` (any more): the player stops a session
 * when it is left or replaced, and the server then deletes it.
 */
export async function expectSessionDeleted(page: Page, sessionId: string): Promise<void> {
  await expect
    .poll(async () => (await page.request.get(`${sessionsPath}/${sessionId}`)).status())
    .toBe(notFoundStatus);
}

const playlistPattern = "**/api/v1/playback/sessions/*/hls/index.m3u8";
const endListTag = "#EXT-X-ENDLIST";

/** The first `listedSegments` segments of an HLS media playlist, without its end tag. */
function truncatePlaylist(playlist: string, listedSegments: number): string {
  const kept: string[] = [];
  let segments = 0;
  for (const line of playlist.split(/\r?\n/)) {
    if (segments === listedSegments || line.startsWith(endListTag)) {
      break;
    }
    kept.push(line);
    // In a media playlist every line that is neither empty nor a tag is a segment URI.
    if (line !== "" && !line.startsWith("#")) {
      segments += 1;
    }
  }
  return `${kept.join("\n")}\n`;
}

/**
 * Makes the first HLS session `page` opens look like a remux still in progress: its playlist
 * lists only the first `listedSegments` segments and no end. A test clip is remuxed within a
 * second, after which the player seeks inside it locally (ADR 0024); on a slow server or with
 * a long film the target is often not produced yet, and only then does a seek restart the
 * session. Later sessions are served as they are.
 */
export async function holdBackFirstStream(page: Page, listedSegments: number): Promise<void> {
  let firstSessionId: string | undefined;
  await page.route(playlistPattern, async (route) => {
    // .../sessions/<id>/hls/index.m3u8
    const sessionId = new URL(route.request().url()).pathname.split("/").at(-3);
    firstSessionId ??= sessionId;
    const response = await route.fetch();
    if (sessionId !== firstSessionId || !response.ok()) {
      await route.fulfill({ response });
      return;
    }
    await route.fulfill({
      response,
      body: truncatePlaylist(await response.text(), listedSegments),
    });
  });
}

/** Leaves the player for the item page and expects the session to be deleted. */
export async function leavePlayer(page: Page, sessionId: string): Promise<void> {
  await page.goBack();
  await expect(page.getByRole("link", { name: "Play" })).toBeVisible();
  await expectSessionDeleted(page, sessionId);
}

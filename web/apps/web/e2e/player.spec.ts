import { AxeBuilder } from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

import {
  fulfillProblem,
  fulfillSession,
  mockFileId,
  mockItemId,
  mockPlayback,
  mockSession,
} from "./support/playback";

// The player engine against mocked playback endpoints (task M2-27): error and waiting states
// the real server cannot be made to produce on demand. Real playback: playback.spec.ts and
// branded.spec.ts (M2-34).

const playerPath = `/items/${mockItemId}/play`;
const hlsChunk = /\/assets\/hls-[^/]*\.js$/;

/** Collects the URLs of the scripts the page loads, to tell whether hls.js was fetched. */
function scriptUrls(page: Page): string[] {
  const urls: string[] = [];
  page.on("request", (request) => {
    if (request.resourceType() === "script") {
      urls.push(request.url());
    }
  });
  return urls;
}

test("plays a direct-play session from the item page and stops it when leaving", async ({
  page,
}) => {
  const scripts = scriptUrls(page);
  // The file request never answers: the spec checks the wiring, not the media.
  await page.route("**/api/v1/playback/sessions/*/file", () => undefined);
  const mocks = await mockPlayback(page, (route) =>
    fulfillSession(route, mockSession("direct-1", "file")),
  );
  await page.goto(`/items/${mockItemId}`);

  await page.getByRole("link", { name: "Play" }).click();

  const video = page.getByLabel("Video");
  await expect(video).toHaveAttribute("src", "/api/v1/playback/sessions/direct-1/file");
  expect(mocks.sessionRequests).toHaveLength(1);
  const [sessionRequest] = mocks.sessionRequests;
  expect(sessionRequest?.fileId).toBe(mockFileId);
  expect(sessionRequest?.startPositionSeconds).toBeUndefined();
  expect(sessionRequest?.profile.name).not.toBe("");
  // ADR 0023 (decision 2026-10-30): Chromium has MSE, so HLS goes through hls.js.
  expect(sessionRequest?.profile.hls.delivery).toBe("mse");

  await page.goBack();

  await expect(page.getByRole("link", { name: "Play" })).toBeVisible();
  await expect.poll(() => mocks.stoppedSessions).toEqual(["direct-1"]);
  expect(scripts.filter((url) => hlsChunk.test(url))).toEqual([]);
});

test("loads hls.js for an HLS session and waits while the stream is not ready", async ({
  page,
}) => {
  const scripts = scriptUrls(page);
  let playlistRequests = 0;
  await page.route("**/api/v1/playback/sessions/hls-1/hls/index.m3u8", (route) => {
    playlistRequests += 1;
    return fulfillProblem(route, 503, "stream_not_ready", 1);
  });
  await mockPlayback(page, (route) => fulfillSession(route, mockSession("hls-1", "hls")));

  await page.goto(`${playerPath}?t=30`);

  await expect(page.getByRole("status")).toHaveText("The server is preparing the stream…");
  // hls.js does not retry a 503 itself; the engine loads the playlist again after Retry-After.
  await expect.poll(() => playlistRequests, { timeout: 10_000 }).toBeGreaterThanOrEqual(3);
  expect(scripts.filter((url) => hlsChunk.test(url))).toHaveLength(1);
});

test("explains a full server and creates the session again on retry", async ({ page }) => {
  await page.route("**/api/v1/playback/sessions/*/file", () => undefined);
  const mocks = await mockPlayback(page, (route, index) =>
    index === 0
      ? fulfillProblem(route, 503, "session_limit_reached", 5)
      : fulfillSession(route, mockSession("direct-2", "file")),
  );
  await page.goto(`${playerPath}?t=45`);

  await expect(page.getByRole("alert")).toHaveText(
    "Too many videos are playing on the server right now. Try again in a moment.",
  );
  // The retry button takes focus, so keyboard users can act on the error right away.
  await expect(page.getByRole("button", { name: "Try again" })).toBeFocused();
  await page.keyboard.press("Enter");

  await expect(page.getByLabel("Video")).toHaveAttribute(
    "src",
    "/api/v1/playback/sessions/direct-2/file",
  );
  expect(mocks.sessionRequests.map((request) => request.startPositionSeconds)).toEqual([45, 45]);
});

/** A remux of an MKV, as the server answers for a browser without Matroska support. */
const remuxSession = {
  ...mockSession("remux-1", "hls"),
  sourceContainer: "mkv",
  sourceBitRate: 4_073_000,
  streams: [
    { kind: "video", sourceIndex: 0, action: "copy", sourceCodec: "vp9", targetCodec: "vp9" },
    { kind: "audio", sourceIndex: 1, action: "copy", sourceCodec: "opus", targetCodec: "opus" },
  ],
  reasons: [{ code: "container_not_supported", scope: "container" }],
} satisfies ReturnType<typeof mockSession>;

/** Opens the player on a remux whose stream never becomes ready (the spec checks the UI). */
async function openRemux(page: Page): Promise<void> {
  await page.route("**/api/v1/playback/sessions/remux-1/hls/**", () => undefined);
  await mockPlayback(page, (route) => fulfillSession(route, remuxSession));
  await page.goto(playerPath);
  await expect(page.getByRole("heading", { level: 1, name: "Mocked Movie" })).toBeVisible();
}

test("shows the stream-info overlay from the keyboard and returns focus when it closes", async ({
  page,
}) => {
  await openRemux(page);

  await page.keyboard.press("i");

  const overlay = page.getByRole("region", { name: "Stream info" });
  await expect(overlay.getByText("Direct Stream")).toBeVisible();
  await expect(overlay.getByText("MKV not supported → MP4")).toBeVisible();
  await expect(overlay.getByText("MKV → MP4")).toBeVisible();
  await expect(overlay.getByText("4.1 Mbit/s")).toBeVisible();
  await expect(overlay.getByRole("heading", { name: "Stream info" })).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(overlay).toBeHidden();
  await expect(page.getByRole("button", { name: "Stream info" })).toBeFocused();
});

test("has no serious or critical axe violations with the overlay open", async ({ page }) => {
  await openRemux(page);
  await page.getByRole("button", { name: "Stream info" }).click();
  await expect(page.getByRole("region", { name: "Stream info" })).toBeVisible();

  const results = await new AxeBuilder({ page }).analyze();
  const blocking = results.violations
    .filter((violation) => violation.impact === "serious" || violation.impact === "critical")
    .map((violation) => `${violation.id}: ${violation.help} (${String(violation.nodes.length)}x)`);

  expect(blocking).toEqual([]);
});

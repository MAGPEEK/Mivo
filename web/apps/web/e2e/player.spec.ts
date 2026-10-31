import { expect, test, type Page } from "@playwright/test";

import {
  fulfillProblem,
  fulfillSession,
  mockFileId,
  mockItemId,
  mockPlayback,
  mockSession,
} from "./support/playback";

// The player engine against mocked playback endpoints (task M2-27). A real-server playback
// flow follows once M2-32 wires the playback API into the binary (M2-34).

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

  await expect(page.getByRole("status")).toHaveText(
    "Too many videos are playing on the server right now. Try again in a moment.",
  );
  await page.getByRole("button", { name: "Try again" }).click();

  await expect(page.getByLabel("Video")).toHaveAttribute(
    "src",
    "/api/v1/playback/sessions/direct-2/file",
  );
  expect(mocks.sessionRequests.map((request) => request.startPositionSeconds)).toEqual([45, 45]);
});

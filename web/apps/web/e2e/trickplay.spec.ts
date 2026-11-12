import type { PlaybackSession } from "@mivo/api-client";
import { expect, test } from "@playwright/test";

import { webmClip } from "./support/corpus";
import { leavePlayer, startPlayback } from "./support/realPlayback";

// The seek-bar preview against the real server (task M4-16d): the player asks for the trickplay
// manifest, the first 404 makes it request the generation once, and the previews appear after
// the server generated the sheets, without a reload.

const testTimeoutMs = 90_000;
const previewTimeoutMs = 60_000;
const okStatus = 200;

test.describe.configure({ timeout: testTimeoutMs });

test("shows the preview tile on the seek bar after the server generated the sheets", async ({
  page,
}) => {
  const generationRequests: string[] = [];
  const manifestStatuses: number[] = [];
  page.on("request", (request) => {
    if (request.method() === "POST" && request.url().endsWith("/trickplay")) {
      generationRequests.push(request.url());
    }
  });
  page.on("response", (response) => {
    if (response.request().method() === "GET" && response.url().endsWith("/trickplay")) {
      manifestStatuses.push(response.status());
    }
  });
  const session: PlaybackSession = await startPlayback(page, webmClip);
  await expect
    .poll(() => manifestStatuses, { timeout: previewTimeoutMs, message: "manifest statuses" })
    .toContain(okStatus);

  const slider = page.getByRole("slider", { name: "Position" });
  const box = await slider.boundingBox();
  if (box === null) {
    throw new Error("the position slider has no box");
  }
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);

  const tile = page.getByTestId("seek-preview-tile");
  await expect(tile).toBeVisible({ timeout: previewTimeoutMs });
  await expect(tile).toHaveCSS("background-image", /\/trickplay\/0\.jpg/);
  expect(generationRequests.length).toBeLessThanOrEqual(1);

  await leavePlayer(page, session.id);
});

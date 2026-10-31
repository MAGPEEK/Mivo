import { expect, test } from "@playwright/test";

import { blockingAxeViolations } from "./support/axe";
import { browserPlays, denyMediaTypesFromNowOn, proprietaryCodecs } from "./support/codecEmulation";
import { mkvClip, webmClip } from "./support/corpus";
import {
  expectMode,
  expectVideoPlays,
  leavePlayer,
  modes,
  openStreamInfo,
  pressPlay,
  problemCode,
  startPlayback,
} from "./support/realPlayback";

// Real playback in the bundled Chromium against the real server (task M2-34). Open codecs
// play everywhere; the proprietary ones are covered by the `branded` project.

const playbackTestTimeoutMs = 30_000;
const unprocessableStatus = 422;
const h264Type = 'video/mp4; codecs="avc1.42E01E"';

test.describe.configure({ timeout: playbackTestTimeoutMs });

test("plays the WebM with open codecs by Direct Play and names the mode", async ({ page }) => {
  const session = await startPlayback(page, webmClip);
  expect(session.mode).toBe("direct_play");

  await expectVideoPlays(page);
  const overlay = await openStreamInfo(page);
  await expectMode(overlay, modes.directPlay);
  expect(await blockingAxeViolations(page)).toEqual([]);

  await leavePlayer(page, session.id);
});

test("explains that a browser without H.264 cannot play the H.264 MKV", async ({ page }) => {
  // Playwright's Chromium decodes H.264 on some hosts (e.g. Windows), so the codec gap of a
  // Chromium without proprietary codecs is emulated; where it has no H.264, nothing changes.
  await page.goto("/");
  test.info().annotations.push({
    type: "codec gap",
    description: (await browserPlays(page, h264Type))
      ? "This Chromium decodes H.264; a Chromium without proprietary codecs is emulated."
      : "This Chromium has no H.264, like a build without proprietary codecs.",
  });
  await denyMediaTypesFromNowOn(page, proprietaryCodecs);

  const response = await pressPlay(page, mkvClip);

  expect(response.status()).toBe(unprocessableStatus);
  expect(await problemCode(response)).toBe("no_compatible_format");
  await expect(page.getByRole("alert")).toHaveText(
    "This browser cannot play this file, not even after converting it.",
  );
});

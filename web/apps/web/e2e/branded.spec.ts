import type { PlaybackSession } from "@mivo/api-client";
import { expect, test } from "@playwright/test";

import { blockingAxeViolations } from "./support/axe";
import { brandedChannel } from "./support/brandedChannel";
import { browserPlays, denyMediaTypesFromNowOn, matroskaTypes } from "./support/codecEmulation";
import { aviClip, flvClip, longMkvClip, mkvClip, mp4Clip, type CorpusClip } from "./support/corpus";
import {
  expectMode,
  expectReason,
  expectSessionDeleted,
  expectVideoPlays,
  filePosition,
  holdBackFirstStream,
  leavePlayer,
  modes,
  nextSessionResponse,
  openStreamInfo,
  startPlayback,
  type Mode,
} from "./support/realPlayback";

// Real playback in an installed Edge or Chrome, which has the proprietary codecs (H.264, AAC)
// of a real user's browser (ADR 0025, OD-7): all three playback modes play here.

const branded = brandedChannel();
const playbackTestTimeoutMs = 45_000;
const half = 0.5;
// Seconds the position must pass the restart position to count as "playback continues".
const continuedSeconds = 1;
// The seek clip has a keyframe every 2 s; a remux restarts at the keyframe before the target.
const keyframeIntervalSeconds = 2;
const matroskaType = "video/x-matroska";

test.describe.configure({ timeout: playbackTestTimeoutMs });
test.skip(!branded.installed, branded.skipReason);

interface ModeCase {
  readonly clip: CorpusClip;
  readonly sessionMode: PlaybackSession["mode"];
  readonly mode: Mode;
  /** One expected reason; undefined for Direct Play, which needs none. */
  readonly reason?: RegExp;
}

const modeCases: readonly ModeCase[] = [
  { clip: mp4Clip, sessionMode: "direct_play", mode: modes.directPlay },
  {
    clip: flvClip,
    sessionMode: "remux",
    mode: modes.remux,
    reason: /^FLV not supported → remux to MP4$/,
  },
  {
    clip: aviClip,
    sessionMode: "transcode",
    mode: modes.transcode,
    reason: /^Video MPEG-4 not supported → H\.264$/,
  },
];

for (const { clip, sessionMode, mode, reason } of modeCases) {
  test(`plays ${clip.relativePath} by ${mode.label} and explains why`, async ({ page }) => {
    const session = await startPlayback(page, clip);
    expect(session.mode).toBe(sessionMode);

    await expectVideoPlays(page);
    const overlay = await openStreamInfo(page);
    await expectMode(overlay, mode);
    if (reason !== undefined) {
      await expectReason(overlay, reason);
    }
    expect(await blockingAxeViolations(page)).toEqual([]);

    await leavePlayer(page, session.id);
  });
}

test("plays the MKV directly where the browser plays Matroska, else by remux", async ({ page }) => {
  // Chrome and Edge play Matroska since version 13x; older ones and other browsers do not.
  await page.goto("/");
  const playsMatroska = await browserPlays(page, matroskaType);
  test.info().annotations.push({
    type: "matroska",
    description: playsMatroska ? "plays Matroska: Direct Play" : "no Matroska: remux",
  });

  const session = await startPlayback(page, mkvClip);

  expect(session.mode).toBe(playsMatroska ? "direct_play" : "remux");
  await expectVideoPlays(page);
  await expectMode(await openStreamInfo(page), playsMatroska ? modes.directPlay : modes.remux);
  await leavePlayer(page, session.id);
});

test("restarts the remux at 50 % on a seek and plays on from there", async ({ page }) => {
  // A browser without Matroska support gets the long MKV as a remux (see the MKV test).
  await denyMediaTypesFromNowOn(page, matroskaTypes);
  // Only the first 6-second segment is "produced", so 50 % of 30 s lies beyond the stream.
  await holdBackFirstStream(page, 1);
  const first = await startPlayback(page, longMkvClip);
  expect(first.mode).toBe("remux");
  await expectVideoPlays(page);
  const duration = first.durationSeconds ?? 0;
  expect(duration).toBeGreaterThan(0);

  // A click in the middle of the slider seeks to half the file.
  const restarted = nextSessionResponse(page);
  await page.getByRole("slider", { name: "Position" }).click();
  const second = (await (await restarted).json()) as PlaybackSession;

  expect(second.id).not.toBe(first.id);
  expect(second.startPositionSeconds).toBeGreaterThanOrEqual(
    duration * half - keyframeIntervalSeconds,
  );
  expect(second.startPositionSeconds).toBeLessThanOrEqual(duration * half);
  await expectSessionDeleted(page, first.id);
  await expect
    .poll(() => filePosition(page), { timeout: playbackTestTimeoutMs / 2 })
    .toBeGreaterThan(second.startPositionSeconds + continuedSeconds);

  await leavePlayer(page, second.id);
});

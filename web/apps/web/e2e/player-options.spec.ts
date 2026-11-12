import type { PlaybackRequest, PlaybackSession } from "@mivo/api-client";
import { expect, test } from "@playwright/test";

import { blockingAxeViolations } from "./support/axe";
import { threeAudioClip } from "./support/corpus";
import { openItem } from "./support/library";
import { leavePlayer, nextSessionResponse, startPlayback } from "./support/realPlayback";

// The playback options panel against the real server (task M4-16b): the audio switch is a
// real restart of the session with the chosen stream, which only the server can answer.

const playerTestTimeoutMs = 30_000;

// Matroska stream indexes of the clip: video 0, English stereo 1, German 5.1 2, commentary 3.
const germanStreamIndex = 2;
const createdStatus = 201;

test.describe.configure({ timeout: playerTestTimeoutMs });

test("switches the audio track in the panel and replaces the session", async ({ page }) => {
  const first = await startPlayback(page, threeAudioClip);
  await page.getByRole("button", { name: "Playback options", exact: true }).click();
  const panel = page.getByRole("dialog", { name: "Playback options" });
  await expect(panel).toBeVisible();
  const audio = panel.getByRole("menu", { name: "Audio" });
  await expect(audio.getByRole("menuitemradio")).toHaveCount(3);
  // The file's default track is the first one and has focus.
  await expect(audio.getByRole("menuitemradio", { checked: true })).toContainText("English");
  await expect(audio.getByRole("menuitemradio", { checked: true })).toBeFocused();

  const restart = nextSessionResponse(page);
  await audio.getByRole("menuitemradio", { name: /^German/ }).click();

  const response = await restart;
  const request = response.request().postDataJSON() as PlaybackRequest;
  expect(request).toMatchObject({
    audioStreamIndex: germanStreamIndex,
    replacesSessionId: first.id,
  });
  await expect(audio.getByRole("menuitemradio", { checked: true })).toContainText("German");
  // The panel stays open after a choice, so the new mode is visible next to it.
  await expect(panel).toBeVisible();
  expect(await blockingAxeViolations(page)).toEqual([]);

  await leavePlayer(page, ((await response.json()) as PlaybackSession).id);
});

test("starts the first session with the audio picked on the detail page (M4-16c)", async ({
  page,
}) => {
  await openItem(page, threeAudioClip);
  const audio = page.getByRole("combobox", { name: "Audio" });
  await expect(audio.locator("option")).toHaveCount(3);
  await audio.selectOption({
    label: await audio.locator("option", { hasText: "German" }).innerText(),
  });
  // The pick is state of the navigation, not part of the URL.
  const response = nextSessionResponse(page);

  await page.getByRole("link", { name: "Play" }).click();

  const answer = await response;
  expect(answer.status(), await answer.text()).toBe(createdStatus);
  const body = answer.request().postDataJSON() as PlaybackRequest;
  expect(body.audioStreamIndex).toBe(germanStreamIndex);
  // The subtitle default follows the shown audio, so the pick is sent with an explicit
  // subtitle choice (the clip has no subtitles: Off).
  expect(body.subtitleTrackId).toBeDefined();
  expect(new URL(page.url()).search).not.toContain("audio");
  await leavePlayer(page, ((await answer.json()) as PlaybackSession).id);
});

test("opens and closes with the keyboard and keeps focus where it belongs", async ({ page }) => {
  const session = await startPlayback(page, threeAudioClip);
  const options = page.getByRole("button", { name: "Playback options", exact: true });

  await options.focus();
  await page.keyboard.press("Enter");

  const panel = page.getByRole("dialog", { name: "Playback options" });
  await expect(panel).toBeVisible();
  await expect(options).toHaveAttribute("aria-expanded", "true");
  // Arrow keys move through the list without switching: no second session is requested.
  let sessionRequests = 0;
  page.on("request", (request) => {
    if (request.method() === "POST" && request.url().endsWith("/api/v1/playback/sessions")) {
      sessionRequests += 1;
    }
  });
  await page.keyboard.press("ArrowDown");
  await expect(
    panel.getByRole("menu", { name: "Audio" }).getByRole("menuitemradio").nth(1),
  ).toBeFocused();
  expect(sessionRequests).toBe(0);

  await page.keyboard.press("Escape");

  await expect(panel).toBeHidden();
  await expect(options).toBeFocused();

  await leavePlayer(page, session.id);
});

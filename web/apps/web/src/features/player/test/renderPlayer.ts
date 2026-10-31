// Renders the player page in the whole app with a fake engine hook. The test file must call
// `vi.mock("../hooks/usePlaybackSession")` itself (Vitest hoists it only there).

import { screen } from "@testing-library/react";
import { vi } from "vitest";

import { renderApp, type RenderedApp } from "../../../test/renderApp";
import { usePlaybackSession } from "../hooks/usePlaybackSession";
import type { PlaybackSnapshot } from "../lib/playbackEngine";
import { createFakePlayback, type FakePlayback } from "./fakePlayback";
import { createPlaybackServer } from "./fakePlaybackServer";
import { playerItemId } from "./playerFixtures";

/** Path of the player for the fixture item. */
export const playerPath = `/items/${playerItemId}/play`;

/** The rendered app with the fake engine and the page's video element. */
export interface RenderedPlayer extends RenderedApp {
  readonly playback: FakePlayback;
  readonly video: HTMLVideoElement;
}

/**
 * Renders the app at the player with the engine hook replaced by a fake that shows `snapshot`.
 * The item comes from the fake server, so the page shows its title. `languages` defaults to
 * English.
 */
export async function renderPlayer(
  snapshot: Partial<PlaybackSnapshot> = {},
  languages: readonly string[] = ["en"],
): Promise<RenderedPlayer> {
  const playback = createFakePlayback(snapshot);
  vi.mocked(usePlaybackSession).mockImplementation(playback.usePlayback);
  const server = createPlaybackServer();
  const app = renderApp(playerPath, { fetch: server.fetch, languages });
  // "Video" in both languages.
  const video = await screen.findByLabelText<HTMLVideoElement>("Video");
  return { ...app, playback, video };
}

// Renders the player page in the whole app with a fake engine hook. The test file must call
// `vi.mock("../hooks/usePlaybackSession")` itself (Vitest hoists it only there).

import { screen } from "@testing-library/react";
import { vi } from "vitest";

import { renderApp, type RenderedApp } from "../../../test/renderApp";
import { usePlaybackSession } from "../hooks/usePlaybackSession";
import type { PlaybackSnapshot } from "../lib/playbackEngine";
import { createFakePlayback, type FakePlayback } from "./fakePlayback";
import { createPlaybackServer, type PlaybackServerOptions } from "./fakePlaybackServer";
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
 * English; `server` changes the fake server, e.g. to open an episode.
 */
export async function renderPlayer(
  snapshot: Partial<PlaybackSnapshot> = {},
  languages: readonly string[] = ["en"],
  server: PlaybackServerOptions = {},
): Promise<RenderedPlayer> {
  const playback = createFakePlayback(snapshot);
  vi.mocked(usePlaybackSession).mockImplementation(playback.usePlayback);
  const playbackServer = createPlaybackServer(server);
  const app = renderApp(playerPath, { fetch: playbackServer.fetch, languages });
  // "Video" in both languages.
  const video = await screen.findByLabelText<HTMLVideoElement>("Video");
  return { ...app, playback, video };
}

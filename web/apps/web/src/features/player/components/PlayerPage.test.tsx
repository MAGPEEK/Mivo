import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import { renderApp } from "../../../test/renderApp";
import {
  createdSession,
  createPlaybackServer,
  problemWithRetryAfter,
  sessionResponse,
} from "../test/fakePlaybackServer";
import { playerItemId } from "../test/playerFixtures";

const playerPath = `/items/${playerItemId}/play`;

beforeEach(() => {
  // jsdom implements neither; the engine calls both.
  vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue(undefined);
  vi.spyOn(HTMLMediaElement.prototype, "load").mockReturnValue(undefined);
});

afterEach(() => {
  // Unmount first: closing the engine empties the element, which calls load().
  cleanup();
  vi.restoreAllMocks();
});

describe("PlayerPage", () => {
  it("plays the item in a labelled video element and says when it is ready", async () => {
    const server = createPlaybackServer();
    renderApp(playerPath, { fetch: server.fetch });

    const video = await screen.findByLabelText("Video");
    expect(await screen.findByRole("status")).toHaveProperty("textContent", "Loading the video…");
    await vi.waitFor(() => {
      expect(video.getAttribute("src")).toBe("/api/v1/playback/sessions/session-1/file");
    });
    fireEvent(video, new Event("loadeddata"));

    expect(await screen.findByText("Ready to play")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Try again" })).toBeNull();
  });

  it("explains a full server in German and retries on request", async () => {
    const server = createPlaybackServer({
      createSession: (request, index) =>
        index === 0
          ? problemWithRetryAfter(503, "session_limit_reached", "5")
          : sessionResponse(createdSession(request, index)),
    });
    renderApp(playerPath, { fetch: server.fetch, languages: ["de"] });

    expect(
      await screen.findByText(
        "Auf dem Server laufen gerade zu viele Wiedergaben. Versuche es gleich noch einmal.",
      ),
    ).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Erneut versuchen" }));

    expect(await screen.findByText("Das Video wird geladen …")).toBeTruthy();
    await vi.waitFor(() => {
      expect(server.sessionRequests).toHaveLength(2);
    });
  });

  it("has no serious accessibility violations in its error state", async () => {
    const server = createPlaybackServer({
      createSession: () => problemWithRetryAfter(503, "session_limit_reached", "5"),
    });
    renderApp(playerPath, { fetch: server.fetch });
    await screen.findByRole("button", { name: "Try again" });

    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });
});

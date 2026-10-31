// Mocks the item and playback endpoints with page.route, so player.spec.ts can check the web
// player engine against the API contract in states the real server does not produce on
// demand (task M2-27). Real playback helpers: realPlayback.ts.

import type { ItemDetail, PlaybackRequest, PlaybackSession, Problem } from "@mivo/api-client";
import type { Page, Request, Route } from "@playwright/test";

/** ID of the mocked item. */
export const mockItemId = "8a9b0c1d-2e3f-4a5b-8c6d-7e8f9a0b1c2d";

/** ID of the mocked item's only file. */
export const mockFileId = "1c2d3e4f-5a6b-4c7d-8e9f-0a1b2c3d4e5f";

const createdStatus = 201;
const noContentStatus = 204;

const mockItem: ItemDetail = {
  id: mockItemId,
  libraryId: "3f6c1d2a-9b8e-4f70-8a1c-2d3e4f5a6b7c",
  title: "Mocked Movie",
  files: [
    {
      id: mockFileId,
      fileName: "Mocked.Movie.mp4",
      sizeBytes: 1_000_000,
      modifiedAt: "2026-10-01T09:30:00Z",
      available: true,
      container: "mp4",
      durationSeconds: 120,
      streams: [],
    },
  ],
};

/** A session of the mocked file with the given ID and delivery. */
export function mockSession(id: string, delivery: PlaybackSession["delivery"]): PlaybackSession {
  return {
    id,
    fileId: mockFileId,
    state: delivery === "file" ? "ready" : "starting",
    mode: delivery === "file" ? "direct_play" : "remux",
    delivery,
    streamUrl: `/api/v1/playback/sessions/${id}/${delivery === "file" ? "file" : "hls/index.m3u8"}`,
    startPositionSeconds: 0,
    durationSeconds: 120,
    sourceContainer: "mp4",
    targetContainer: "mp4",
    streams: [],
    reasons: [],
    warnings: [],
  };
}

/** Answers `route` with RFC 9457 problem details, optionally with `Retry-After`. */
export function fulfillProblem(
  route: Route,
  status: number,
  code: string,
  retryAfterSeconds?: number,
): Promise<void> {
  const problem: Problem = { type: "about:blank", title: code, status, code };
  return route.fulfill({
    status,
    contentType: "application/problem+json",
    headers: retryAfterSeconds === undefined ? {} : { "Retry-After": String(retryAfterSeconds) },
    body: JSON.stringify(problem),
  });
}

/** What the mocked server received. */
export interface PlaybackMocks {
  /** Bodies of `POST /api/v1/playback/sessions`, oldest first. */
  readonly sessionRequests: readonly PlaybackRequest[];
  /** IDs of stopped sessions, oldest first. */
  readonly stoppedSessions: readonly string[];
}

/** Answers the n-th session request (0-based). */
export type SessionAnswer = (route: Route, index: number) => Promise<void>;

/** Answers a session request with `session` (201). */
export function fulfillSession(route: Route, session: PlaybackSession): Promise<void> {
  return route.fulfill({
    status: createdStatus,
    contentType: "application/json",
    body: JSON.stringify(session),
  });
}

function sessionIdOf(request: Request): string {
  return new URL(request.url()).pathname.split("/").at(-1) ?? "";
}

/**
 * Mocks the item, session creation (`answer`), session lookup (ready) and stop (204). Stream
 * URLs are not mocked here; each spec decides what they answer.
 */
export async function mockPlayback(page: Page, answer: SessionAnswer): Promise<PlaybackMocks> {
  const sessionRequests: PlaybackRequest[] = [];
  const stoppedSessions: string[] = [];
  await page.route(`**/api/v1/items/${mockItemId}`, (route) =>
    route.fulfill({ contentType: "application/json", body: JSON.stringify(mockItem) }),
  );
  await page.route("**/api/v1/playback/sessions", (route) => {
    sessionRequests.push(route.request().postDataJSON() as PlaybackRequest);
    return answer(route, sessionRequests.length - 1);
  });
  await page.route("**/api/v1/playback/sessions/*", (route) => {
    const request = route.request();
    const sessionId = sessionIdOf(request);
    if (request.method() === "DELETE") {
      stoppedSessions.push(sessionId);
      return route.fulfill({ status: noContentStatus });
    }
    return route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(mockSession(sessionId, "file")),
    });
  });
  return { sessionRequests, stoppedSessions };
}

// An in-memory server for the playback endpoints, on top of the shared fake API server.

import type { ItemDetail, PlaybackRequest, PlaybackSession } from "@mivo/api-client";

import {
  createFakeServer,
  jsonResponse,
  problemResponse,
  type FakeServer,
} from "../../../test/fakeApi";
import { fakeSession, playerItem, type SessionFields } from "./playerFixtures";

const createdStatus = 201;
const noContentStatus = 204;

/** Decides the answer to the n-th session request (0-based). */
export type CreateSessionResponder = (
  request: PlaybackRequest,
  index: number,
) => Response | Promise<Response>;

/** Options of {@link createPlaybackServer}. */
export interface PlaybackServerOptions {
  readonly item?: ItemDetail;
  /** Delivery of the sessions the default responder creates. */
  readonly delivery?: SessionFields["delivery"];
  /** Replaces the default responder, which creates `session-1`, `session-2`, ... */
  readonly createSession?: CreateSessionResponder;
  /** Answers `GET` of a session; defaults to the session as created. */
  readonly getSession?: (sessionId: string) => Response | Promise<Response>;
}

/** The fake server with what it received. */
export interface PlaybackServer extends FakeServer {
  /** Bodies of every `POST /api/v1/playback/sessions`, oldest first. */
  readonly sessionRequests: readonly PlaybackRequest[];
  /** IDs of every `DELETE` of a session, oldest first. */
  readonly stoppedSessions: readonly string[];
  /** The `DELETE` requests themselves, e.g. to check `keepalive`. */
  readonly stopRequests: readonly Request[];
  /** IDs of every `GET` of a session, oldest first. */
  readonly lookedUpSessions: readonly string[];
}

/** A session created by the default responder: `session-<n>` at the requested start. */
export function createdSession(
  request: PlaybackRequest,
  index: number,
  delivery: SessionFields["delivery"] = "file",
): PlaybackSession {
  return fakeSession({
    id: `session-${String(index + 1)}`,
    delivery,
    startPositionSeconds: request.startPositionSeconds ?? 0,
  });
}

/** A 201 answer with `session`. */
export function sessionResponse(session: PlaybackSession): Response {
  return jsonResponse(session, createdStatus);
}

/** A problem answer with a `Retry-After` header. */
export function problemWithRetryAfter(status: number, code: string, retryAfter: string): Response {
  const response = problemResponse(status, code);
  response.headers.set("Retry-After", retryAfter);
  return response;
}

/** Creates a fake server that knows the player's item and the playback endpoints. */
export function createPlaybackServer(options: PlaybackServerOptions = {}): PlaybackServer {
  const { item = playerItem(), delivery = "file" } = options;
  const sessionRequests: PlaybackRequest[] = [];
  const stoppedSessions: string[] = [];
  const stopRequests: Request[] = [];
  const lookedUpSessions: string[] = [];
  const sessions = new Map<string, PlaybackSession>();
  const createSession: CreateSessionResponder =
    options.createSession ??
    ((request, index) => sessionResponse(createdSession(request, index, delivery)));
  const server = createFakeServer([
    { method: "GET", path: `/api/v1/items/${item.id}`, respond: () => jsonResponse(item) },
    {
      method: "POST",
      path: "/api/v1/playback/sessions",
      respond: async ({ request }) => {
        const body = (await request.json()) as PlaybackRequest;
        sessionRequests.push(body);
        const response = await createSession(body, sessionRequests.length - 1);
        if (response.ok) {
          const session = (await response.clone().json()) as PlaybackSession;
          sessions.set(session.id, session);
        }
        return response;
      },
    },
    {
      method: "GET",
      path: "/api/v1/playback/sessions/:sessionId",
      respond: ({ params }) => {
        const sessionId = params["sessionId"] ?? "";
        lookedUpSessions.push(sessionId);
        if (options.getSession !== undefined) {
          return options.getSession(sessionId);
        }
        const session = sessions.get(sessionId);
        return session === undefined ? problemResponse(404, "not_found") : jsonResponse(session);
      },
    },
    {
      method: "DELETE",
      path: "/api/v1/playback/sessions/:sessionId",
      respond: ({ request, params }) => {
        stoppedSessions.push(params["sessionId"] ?? "");
        stopRequests.push(request);
        return new Response(null, { status: noContentStatus });
      },
    },
  ]);
  return { ...server, sessionRequests, stoppedSessions, stopRequests, lookedUpSessions };
}

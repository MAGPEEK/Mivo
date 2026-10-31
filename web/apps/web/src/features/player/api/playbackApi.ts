import {
  ApiError,
  clientErrorCodes,
  noResponseStatus,
  requestData,
  type ApiClient,
  type PlaybackRequest,
  type RequestResult,
} from "@mivo/api-client";
import type { PlaybackSession } from "@mivo/player-core";

import { PlaybackRequestError } from "../lib/playbackErrors";
import type { PlaybackApi, SessionRequest } from "../lib/playbackPorts";
import { parseRetryAfter, retryAfterHeader } from "../lib/retryAfter";

const sessionsPath = "/api/v1/playback/sessions";
const sessionPath = "/api/v1/playback/sessions/{sessionId}";

function toApiRequest(request: SessionRequest): PlaybackRequest {
  const body: PlaybackRequest = { fileId: request.fileId, profile: request.profile };
  if (request.startPositionSeconds > 0) {
    body.startPositionSeconds = request.startPositionSeconds;
  }
  if (request.replacesSessionId !== undefined) {
    body.replacesSessionId = request.replacesSessionId;
  }
  return body;
}

async function retryAfterOf(call: Promise<RequestResult<unknown>>): Promise<number | undefined> {
  const result = await call.catch(() => undefined);
  return parseRetryAfter(result?.response.headers.get(retryAfterHeader));
}

function toRequestError(error: unknown, retryAfterSeconds?: number): PlaybackRequestError {
  const failure =
    error instanceof ApiError
      ? error
      : { status: noResponseStatus, code: clientErrorCodes.networkError };
  return new PlaybackRequestError(
    { status: failure.status, code: failure.code, retryAfterSeconds },
    { cause: error },
  );
}

/**
 * The playback endpoints through the generated API client (ADR 0017). Sessions come back as
 * player-core types, which the API types are checked against at compile time (apiTypeChecks).
 */
export function createPlaybackApi(client: ApiClient): PlaybackApi {
  return {
    async createSession(request) {
      const call = client.POST(sessionsPath, { body: toApiRequest(request) });
      try {
        const session: PlaybackSession = await requestData(call);
        return session;
      } catch (error) {
        // session_limit_reached comes with Retry-After, which the problem body does not carry.
        throw toRequestError(error, await retryAfterOf(call));
      }
    },
    async getSession(sessionId) {
      try {
        const session: PlaybackSession = await requestData(
          client.GET(sessionPath, { params: { path: { sessionId } } }),
        );
        return session;
      } catch (error) {
        throw toRequestError(error);
      }
    },
    stopSession(sessionId) {
      // keepalive lets the request outlive the page on pagehide and navigation (ADR 0024: an
      // abandoned session would otherwise run until its idle timeout). DELETE is idempotent,
      // so a failure is not worth reporting.
      client
        .DELETE(sessionPath, { params: { path: { sessionId } }, keepalive: true })
        .catch(() => undefined);
    },
  };
}

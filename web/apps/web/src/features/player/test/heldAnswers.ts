// Session requests a test answers one by one, to control what is in flight when.

import type { PlaybackRequest } from "@mivo/api-client";
import { expect, vi } from "vitest";

import { problemResponse } from "../../../test/fakeApi";
import { createdSession, sessionResponse, type CreateSessionResponder } from "./fakePlaybackServer";

/** Session requests held until the test answers them. */
export interface HeldAnswers {
  /** Pass as the fake server's `createSession`. */
  readonly respond: CreateSessionResponder;
  /** Answers the n-th request (0-based) with `session-<n+1>` once it has arrived. */
  readonly release: (index: number) => Promise<void>;
  /** Answers the n-th request (0-based) with a 500 problem once it has arrived. */
  readonly fail: (index: number) => Promise<void>;
  /** The most requests that were in flight at the same time. */
  readonly maxInFlight: () => number;
}

interface HeldAnswer {
  readonly release: () => void;
  readonly fail: () => void;
}

/** Creates {@link HeldAnswers} whose sessions have the given delivery. */
export function createHeldAnswers(delivery: "file" | "hls"): HeldAnswers {
  const held: HeldAnswer[] = [];
  let inFlight = 0;
  let maxInFlight = 0;
  const respond = (request: PlaybackRequest, index: number) =>
    new Promise<Response>((resolve) => {
      inFlight += 1;
      maxInFlight = Math.max(maxInFlight, inFlight);
      const settle = (response: Response) => {
        inFlight -= 1;
        resolve(response);
      };
      held[index] = {
        release: () => {
          settle(sessionResponse(createdSession(request, index, delivery)));
        },
        fail: () => {
          settle(problemResponse(500, "internal"));
        },
      };
    });
  const answer = async (index: number, how: "release" | "fail") => {
    await vi.waitFor(() => {
      expect(held[index]).toBeDefined();
    });
    held[index]?.[how]();
  };
  return {
    respond,
    release: (index) => answer(index, "release"),
    fail: (index) => answer(index, "fail"),
    maxInFlight: () => maxInFlight,
  };
}

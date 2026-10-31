// Compile-time proofs that player-core's hand-written mirrors of API and hls.js types stay
// compatible (ADR 0023, task M2-27). player-core may not import the API client
// (dependency-cruiser rule player-core-not-to-api-client) nor hls.js, so the web app checks the
// structures here. `tsc -b` (pnpm lint) fails when a side drifts; nothing here exists at run time
// except the `true` values that carry the checks.

import type * as Api from "@mivo/api-client";
import type * as Core from "@mivo/player-core";
import type { ErrorData } from "hls.js";

/** Resolves to `true` when `Source` is assignable to `Target`, otherwise to `never`. */
type Assignable<Source, Target> = [Source] extends [Target] ? true : never;

/**
 * Resolves to `true` when both types declare the same property names. Assignability alone
 * lets an optional property exist on one side only; this catches it.
 */
type SameKeys<First, Second> = [keyof First] extends [keyof Second]
  ? [keyof Second] extends [keyof First]
    ? true
    : never
  : never;

/** The profile player-core detects is what `POST /api/v1/playback/sessions` accepts. */
export const clientProfileFitsApi: Assignable<Core.ClientProfile, Api.ClientProfile> = true;

/** Profile parts name the same fields on both sides, optional ones included. */
export const clientProfileKeysMatch: [
  SameKeys<Core.ClientProfile, Api.ClientProfile>,
  SameKeys<Core.DirectPlayProfile, Api.DirectPlayProfile>,
  SameKeys<Core.HlsProfile, Api.HlsProfile>,
  SameKeys<Core.CodecLimit, Api.CodecLimit>,
] = [true, true, true, true];

/** A session from the API is a player-core session ... */
export const apiSessionFitsCore: Assignable<Api.PlaybackSession, Core.PlaybackSession> = true;

/** ... and the other way round, so no value type is wider on either side ... */
export const coreSessionFitsApi: Assignable<Core.PlaybackSession, Api.PlaybackSession> = true;

/** ... and every session part names the same fields, optional ones included. */
export const sessionKeysMatch: [
  SameKeys<Core.PlaybackSession, Api.PlaybackSession>,
  SameKeys<Core.StreamDecision, Api.StreamDecision>,
  SameKeys<Core.PlaybackReason, Api.PlaybackReason>,
] = [true, true, true];

/** hls.js passes its `ErrorData` to the engine as player-core's `HlsErrorData`. */
export const hlsErrorFitsCore: Assignable<ErrorData, Core.HlsErrorData> = true;

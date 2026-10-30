// Reasons and warnings of a session as translation keys with params. The params carry the
// technical labels of the part of the file a code concerns, so a text can read "Audio AC-3 is
// not supported, converted to AAC" without the app looking up streams itself.

import { codecLabel, containerLabel } from "../labels/formatLabels";
import { channelLabel } from "../labels/streamLabels";
import type { PlaybackReason, PlaybackSession } from "./playbackSession";
import { targetResolution } from "./streamLines";
import {
  keyLookup,
  knownReasonCodes,
  knownWarningCodes,
  type ReasonKey,
  type WarningKey,
} from "./translationKeys";

/** Interpolation values of a translated text. */
export type TextParams = Readonly<Record<string, string>>;

/**
 * A reason or warning ready to translate. Params always contain `code` (the raw code, useful
 * in the generic text for unknown codes) and, by scope:
 * - `container`: `sourceContainer` and `targetContainer` (labels, e.g. `MKV`, `MP4`);
 * - `video`, `audio`, `subtitle`: `sourceCodec` and `targetCodec` of the session's first stream
 *   of that kind, plus `resolution` (video) or `channels` (audio) when the target has them.
 *   Without such a stream there are no further params.
 */
export interface ReasonText<Key extends ReasonKey | WarningKey = ReasonKey | WarningKey> {
  readonly code: string;
  /** Scope as the server sent it, e.g. `audio`. */
  readonly scope: string;
  readonly key: Key;
  readonly params: TextParams;
}

/** The parts of a session reasons, warnings and their params come from. */
export type ReasonSession = Pick<
  PlaybackSession,
  "sourceContainer" | "targetContainer" | "streams" | "reasons" | "warnings"
>;

const reasonKey = keyLookup("reason", knownReasonCodes);
const warningKey = keyLookup("warning", knownWarningCodes);

/** The reasons of a session, in server order; unknown codes get `streamInfo.reason.unknown`. */
export function reasonTexts(session: ReasonSession): ReasonText<ReasonKey>[] {
  return session.reasons.map((reason) => toText(session, reason, reasonKey));
}

/** The warnings of a session, in server order; unknown codes get `streamInfo.warning.unknown`. */
export function warningTexts(session: ReasonSession): ReasonText<WarningKey>[] {
  return session.warnings.map((warning) => toText(session, warning, warningKey));
}

function toText<Key extends ReasonKey | WarningKey>(
  session: ReasonSession,
  reason: PlaybackReason,
  keyOf: (code: string) => Key,
): ReasonText<Key> {
  return {
    code: reason.code,
    scope: reason.scope,
    key: keyOf(reason.code),
    params: { code: reason.code, ...scopeParams(session, reason.scope) },
  };
}

function scopeParams(session: ReasonSession, scope: string): TextParams {
  if (scope === "container") {
    return {
      sourceContainer: containerLabel(session.sourceContainer),
      targetContainer: containerLabel(session.targetContainer),
    };
  }
  const stream = session.streams.find((decision) => decision.kind === scope);
  if (stream === undefined) {
    return {};
  }
  const resolution = targetResolution(stream);
  return {
    sourceCodec: codecLabel(stream.sourceCodec),
    targetCodec: codecLabel(stream.targetCodec),
    ...(resolution === undefined ? {} : { resolution }),
    ...(stream.targetChannels === undefined
      ? {}
      : { channels: channelLabel(stream.targetChannels) }),
  };
}

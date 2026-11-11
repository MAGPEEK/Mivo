// Builds the body of a playback request (ADR 0030, API 1.16.0). `PlaybackRequestBody` is
// structurally identical to the API schema `PlaybackRequest`, written out here because
// player-core must not depend on the generated client.

import type { ClientProfile } from "../profile/clientProfile";
import { restartPosition } from "../seek/planSeek";
import type { PlaybackSession } from "../streamInfo/playbackSession";
import { displayedPosition } from "../timeline/timeline";

/** `subtitleTrackId` that switches subtitles off; an omitted id lets the server's policy pick. */
export const subtitlesOffTrackId = "";

/** `maxBitRate` of the original quality; it is never sent, an omitted limit means the same. */
export const originalQualityBitRate = 0;

// A track id of the API: "s:<stream index>" for an embedded stream, "x:<hash>" for a file.
const subtitleTrackIdPattern = /^(?:s:[0-9]{1,9}|x:[0-9a-f]{1,64})$/;

/** What the viewer chose for a playback; every omitted field is left to the server. */
export interface TrackSelection {
  /** `MediaStream.index` of the audio track; omitted applies the server's default policy. */
  readonly audioStreamIndex?: number | undefined;
  /**
   * `SubtitleTrack.id` of the subtitle; {@link subtitlesOffTrackId} switches subtitles off and
   * omitted applies the server's default policy.
   */
  readonly subtitleTrackId?: string | undefined;
  /** Upper limit of the stream in bits per second; omitted or 0 plays the original quality. */
  readonly maxBitRate?: number | undefined;
}

/** Body of `POST /playback/sessions`. */
export interface PlaybackRequestBody {
  fileId: string;
  profile: ClientProfile;
  startPositionSeconds?: number;
  audioStreamIndex?: number;
  subtitleTrackId?: string;
  allowDegradeSubtitles?: boolean;
  maxBitRate?: number;
  replacesSessionId?: string;
}

/** What a playback request is built from. */
export interface SessionRequestOptions {
  readonly fileId: string;
  readonly profile: ClientProfile;
  /** File position to start at, in seconds; omitted or 0 starts at the beginning. */
  readonly startPositionSeconds?: number | undefined;
  readonly selection?: TrackSelection | undefined;
  /** The running session this one replaces: a seek, a track or a quality switch. */
  readonly replacesSessionId?: string | undefined;
}

/** Whether the id names a track, as opposed to "off", "policy" or something malformed. */
export function isSubtitleTrackId(id: string | undefined): id is string {
  return id !== undefined && subtitleTrackIdPattern.test(id);
}

/**
 * The request for a session. Values the API would reject are left out rather than sent, so
 * the server's policy applies instead of a 400: a negative or fractional audio index, a
 * malformed subtitle id, a bit rate that is no positive number.
 *
 * A request that replaces a running session and names a subtitle carries
 * `allowDegradeSubtitles: true`: when the subtitle needs burn-in and the server cannot burn in
 * now, playback continues without it instead of stopping over a subtitle (ADR 0030 Notes
 * M4-02b). The first request of a playback does not, so the viewer learns why nothing starts.
 */
export function buildSessionRequest(options: SessionRequestOptions): PlaybackRequestBody {
  const { fileId, profile, startPositionSeconds, selection = {}, replacesSessionId } = options;
  const { audioStreamIndex, subtitleTrackId, maxBitRate } = selection;
  const body: PlaybackRequestBody = { fileId, profile };
  if (startPositionSeconds !== undefined && startPositionSeconds > 0) {
    body.startPositionSeconds = startPositionSeconds;
  }
  if (
    audioStreamIndex !== undefined &&
    Number.isInteger(audioStreamIndex) &&
    audioStreamIndex >= 0
  ) {
    body.audioStreamIndex = audioStreamIndex;
  }
  if (subtitleTrackId === subtitlesOffTrackId || isSubtitleTrackId(subtitleTrackId)) {
    body.subtitleTrackId = subtitleTrackId;
  }
  if (replacesSessionId !== undefined && isSubtitleTrackId(subtitleTrackId)) {
    body.allowDegradeSubtitles = true;
  }
  if (maxBitRate !== undefined && Number.isFinite(maxBitRate) && maxBitRate >= 1) {
    body.maxBitRate = Math.floor(maxBitRate);
  }
  if (replacesSessionId !== undefined) {
    body.replacesSessionId = replacesSessionId;
  }
  return body;
}

/** The running session a switch restarts. */
export type RestartableSession = Pick<
  PlaybackSession,
  "id" | "fileId" | "delivery" | "startPositionSeconds" | "durationSeconds"
>;

/** What a restart at the current position is built from. */
export interface RestartRequestOptions {
  readonly session: RestartableSession;
  /** The media element's `currentTime`. */
  readonly currentTime: number;
  readonly profile: ClientProfile;
  /** The whole selection after the switch, not only what changed. */
  readonly selection: TrackSelection;
}

/**
 * The request that replaces a running session at the position it plays now, for a switch of
 * the audio track, of a burned-in subtitle or of the quality. The position goes through
 * `restartPosition`, like a seek by restart, so it never lands at or after the end of the file.
 */
export function buildRestartRequest(options: RestartRequestOptions): PlaybackRequestBody {
  const { session, currentTime, profile, selection } = options;
  return buildSessionRequest({
    fileId: session.fileId,
    profile,
    startPositionSeconds: restartPosition(session, displayedPosition(session, currentTime)),
    selection,
    replacesSessionId: session.id,
  });
}

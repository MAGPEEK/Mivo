// How the item detail page hands the viewer's audio and subtitle pick to the player: in the
// history state of the navigation, not the URL, next to the queue (queueHandoff.ts). The
// player reads it once when it opens an item; a reload keeps it, a plain link carries none.
//
// History state is user-controllable, so it is validated strictly: a state with any invalid
// field is ignored as a whole and the server's default policy plays.

import { isSubtitleTrackId, subtitlesOffTrackId, type TrackSelection } from "@mivo/player-core";

import type { PlayerTracks } from "./playerTracks";

/** The tracks the viewer picked; a field is omitted when the viewer left it to the policy. */
export interface PlayChoice {
  /** `MediaStream.index` of the audio track. */
  readonly audioStreamIndex?: number;
  /** `SubtitleTrack.id`, or the empty string for "Off". */
  readonly subtitleTrackId?: string;
}

const handoffKey = "mivoPlayChoice";

/** Highest stream index accepted; far above any real file. */
export const maxAudioStreamIndex = 999;
/** Longest subtitle track id accepted (the API's `s:<n>` and `x:<hash>` forms are far shorter). */
export const maxSubtitleTrackIdLength = 128;

/**
 * The history state that carries `choice` to the player route of `itemId`; `undefined` for a
 * choice that names nothing. The item is part of the state so that it applies to that item
 * only, not to the next one of a queue.
 */
export function playChoiceState(
  itemId: string,
  choice: PlayChoice,
): Record<string, unknown> | undefined {
  if (choice.audioStreamIndex === undefined && choice.subtitleTrackId === undefined) {
    return undefined;
  }
  return { [handoffKey]: { itemId, ...choice } };
}

function isAudioStreamIndex(value: unknown): value is number {
  // Negative zero passes the range check but is no index a client writes.
  return (
    Number.isInteger(value) &&
    (value as number) >= 0 &&
    (value as number) <= maxAudioStreamIndex &&
    !Object.is(value, -0)
  );
}

function isSubtitleChoice(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value.length <= maxSubtitleTrackIdLength &&
    (value === subtitlesOffTrackId || isSubtitleTrackId(value))
  );
}

/**
 * The choice the history state carries for `itemId`; `{}` when there is none, it is malformed
 * or it was made for another item (state that outlived its navigation).
 */
export function readPlayChoice(state: unknown, itemId: string): PlayChoice {
  if (typeof state !== "object" || state === null) {
    return {};
  }
  const handoff: unknown = (state as Record<string, unknown>)[handoffKey];
  if (typeof handoff !== "object" || handoff === null) {
    return {};
  }
  const fields = handoff as Record<string, unknown>;
  const { audioStreamIndex, subtitleTrackId } = fields;
  const valid =
    fields["itemId"] === itemId &&
    (audioStreamIndex === undefined || isAudioStreamIndex(audioStreamIndex)) &&
    (subtitleTrackId === undefined || isSubtitleChoice(subtitleTrackId));
  if (!valid) {
    return {};
  }
  return {
    ...(isAudioStreamIndex(audioStreamIndex) ? { audioStreamIndex } : {}),
    ...(isSubtitleChoice(subtitleTrackId) ? { subtitleTrackId } : {}),
  };
}

/**
 * The first session's track selection: the choice, limited to tracks the file has. An unknown
 * track is dropped, which leaves that track to the policy.
 */
export function choiceSelection(
  choice: PlayChoice | undefined,
  tracks: Pick<PlayerTracks, "audio" | "subtitles"> | undefined,
): TrackSelection {
  if (choice === undefined || tracks === undefined) {
    return {};
  }
  const { audioStreamIndex, subtitleTrackId } = choice;
  const hasAudio =
    audioStreamIndex !== undefined &&
    tracks.audio.some((track) => track.index === audioStreamIndex);
  const hasSubtitle =
    subtitleTrackId !== undefined &&
    (subtitleTrackId === subtitlesOffTrackId ||
      tracks.subtitles.some((track) => track.id === subtitleTrackId));
  return {
    ...(hasAudio ? { audioStreamIndex } : {}),
    ...(hasSubtitle ? { subtitleTrackId } : {}),
  };
}

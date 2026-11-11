// What a switch during playback costs (ADR 0030 §3, §4, §6): a text subtitle is swapped by the
// client and never touches the stream; audio, quality and anything involving a burned-in
// subtitle replace the session at the current position.

import { originalQualityBitRate, subtitlesOffTrackId, type TrackSelection } from "./sessionRequest";
import {
  sessionAudioIndex,
  sessionSubtitle,
  type SubtitleTrack,
  type TrackSession,
} from "../tracks/trackModel";

/** Nothing changes: the choice is what already plays. */
export interface NoSwitch {
  readonly kind: "none";
}

/**
 * The client swaps the subtitle it renders; the stream keeps playing. `track` is `undefined`
 * for "off". `selection` is to be remembered, so a later restart asks for the same subtitle.
 */
export interface SidecarSwitch {
  readonly kind: "sidecar";
  readonly track: SubtitleTrack | undefined;
  readonly selection: TrackSelection;
}

/** Replace the session at the current position with `selection` (`buildRestartRequest`). */
export interface RestartSwitch {
  readonly kind: "restart";
  readonly selection: TrackSelection;
}

/** How to carry out a switch. */
export type SwitchPlan = NoSwitch | SidecarSwitch | RestartSwitch;

/** What plays at the moment of a switch. */
export interface SwitchState {
  /** What the viewer chose so far; fields the server's policy decided are omitted. */
  readonly selection: TrackSelection;
  readonly session: TrackSession;
}

const noSwitch: NoSwitch = { kind: "none" };

/** Switches the audio track; a change always replaces the session (ADR 0024). */
export function planAudioSwitch(state: SwitchState, audioStreamIndex: number): SwitchPlan {
  const current = state.selection.audioStreamIndex ?? sessionAudioIndex(state.session);
  if (audioStreamIndex === current) {
    return noSwitch;
  }
  return { kind: "restart", selection: { ...state.selection, audioStreamIndex } };
}

/**
 * Switches the quality to `maxBitRate` bits per second ({@link originalQualityBitRate} for the
 * original); a change always replaces the session, because the server re-encodes for it.
 */
export function planQualitySwitch(state: SwitchState, maxBitRate: number): SwitchPlan {
  const current = state.selection.maxBitRate ?? originalQualityBitRate;
  if (maxBitRate === current) {
    return noSwitch;
  }
  return { kind: "restart", selection: { ...state.selection, maxBitRate } };
}

/** Id of the subtitle shown now: the viewer's choice, else what the session's policy picked. */
export function shownSubtitleTrackId(state: SwitchState): string {
  const chosen = state.selection.subtitleTrackId;
  if (chosen !== undefined) {
    return chosen;
  }
  const { mode, trackId } = sessionSubtitle(state.session);
  return mode === "off" || trackId === undefined ? subtitlesOffTrackId : trackId;
}

/**
 * Switches the subtitle to the track with `trackId`, or off with {@link subtitlesOffTrackId}.
 * Only a change from or to a burned-in subtitle restarts the stream; between text tracks and
 * to "off" the client swaps what it renders. An id that names no track of the file changes
 * nothing.
 */
export function planSubtitleSwitch(
  state: SwitchState,
  tracks: readonly SubtitleTrack[],
  trackId: string,
): SwitchPlan {
  const track = tracks.find((candidate) => candidate.id === trackId);
  if (
    trackId === shownSubtitleTrackId(state) ||
    (track === undefined && trackId !== subtitlesOffTrackId)
  ) {
    return noSwitch;
  }
  const selection: TrackSelection = { ...state.selection, subtitleTrackId: trackId };
  const burnedIn = sessionSubtitle(state.session).mode === "burn_in";
  if (burnedIn || (track !== undefined && !track.sidecar)) {
    return { kind: "restart", selection };
  }
  return { kind: "sidecar", track, selection };
}

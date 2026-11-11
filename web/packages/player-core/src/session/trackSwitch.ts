// What a switch during playback costs (ADR 0030 §3, §4, §6): a text subtitle is swapped by the
// client and never touches the stream; audio, quality and anything involving a burned-in
// subtitle replace the session at the current position.

import { originalQualityBitRate, subtitlesOffTrackId, type TrackSelection } from "./sessionRequest";
import {
  sessionAudioIndex,
  sessionSubtitle,
  type AudioTrack,
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

/**
 * Switches the audio track to the stream with `audioStreamIndex`; a change always replaces the
 * session (ADR 0024). An index that names no audio track of the file changes nothing, because
 * the server would answer it with 400.
 */
export function planAudioSwitch(
  state: SwitchState,
  tracks: readonly AudioTrack[],
  audioStreamIndex: number,
): SwitchPlan {
  const current = state.selection.audioStreamIndex ?? sessionAudioIndex(state.session);
  const isTrack = tracks.some((track) => track.index === audioStreamIndex);
  if (audioStreamIndex === current || !isTrack) {
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

/**
 * The selection as the session carries it out. A subtitle that needs burn-in is only shown
 * when the session burns it in; when the server could not (it starts such a session with
 * subtitles off and the warning `subtitle_burn_in_unavailable`, because requests during
 * playback allow that), the choice becomes "off". Without this the menu would mark a subtitle
 * nobody sees, and choosing it again would do nothing. Text tracks are rendered by the client
 * whatever the session says, so they stay chosen. The player keeps the result as its
 * selection after every session start, so later restarts do not ask for the burn-in again.
 * Returns the same object when nothing changes.
 */
export function reconcileSelection(
  selection: TrackSelection,
  session: TrackSession,
  tracks: readonly SubtitleTrack[],
): TrackSelection {
  const chosen = tracks.find((track) => track.id === selection.subtitleTrackId);
  if (chosen === undefined || chosen.sidecar) {
    return selection;
  }
  const { mode, trackId } = sessionSubtitle(session);
  if (mode === "burn_in" && trackId === chosen.id) {
    return selection;
  }
  return { ...selection, subtitleTrackId: subtitlesOffTrackId };
}

/**
 * Id of the subtitle shown now: the viewer's choice as far as the session carries it out
 * ({@link reconcileSelection}), else what the session's policy picked.
 */
export function shownSubtitleTrackId(state: SwitchState, tracks: readonly SubtitleTrack[]): string {
  const chosen = reconcileSelection(state.selection, state.session, tracks).subtitleTrackId;
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
 * nothing. A burn-in the server could not do counts as off, so choosing that track again asks
 * the server once more.
 */
export function planSubtitleSwitch(
  state: SwitchState,
  tracks: readonly SubtitleTrack[],
  trackId: string,
): SwitchPlan {
  const track = tracks.find((candidate) => candidate.id === trackId);
  if (
    trackId === shownSubtitleTrackId(state, tracks) ||
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

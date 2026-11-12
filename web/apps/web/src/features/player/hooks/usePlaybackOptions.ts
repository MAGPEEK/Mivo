import {
  planAudioSwitch,
  planQualitySwitch,
  planSubtitleSwitch,
  shownSubtitleTrackId,
  subtitlesOffTrackId,
  type PlaybackSession,
  type SubtitleTrack,
  type SwitchState,
} from "@mivo/player-core";
import { useRef, useState, type RefObject } from "react";

import { browserLocalStorage } from "../../../shared/lib/persistentFlag";
import {
  activeStep,
  audioRows,
  currentAudioIndex,
  qualityRows,
  subtitleIsBurnedIn,
  subtitleRows,
  visibleQualityRows,
  type AudioRow,
  type QualityRow,
  type SubtitleRow,
} from "../lib/optionRows";
import {
  availableQualityChoice,
  readQualityChoice,
  startingMaxBitRate,
  writeQualityChoice,
  type QualityChoice,
} from "../lib/qualityPreference";
import type { PlaybackSpeed } from "../lib/playbackSpeed";
import { sessionToasts } from "../lib/sessionToasts";
import {
  readSubtitleSize,
  writeSubtitleSize,
  type SubtitleSize,
} from "../lib/subtitleSizePreference";
import type { PlaybackPreferences } from "./usePlaybackPreferences";
import type { PlaybackSessionView } from "./usePlaybackSession";

/** What the playback options panel shows (PL-10 … PL-13). */
export interface PlaybackOptionsView {
  readonly open: boolean;
  /** Audio tracks in file order; empty until the item is loaded. */
  readonly audio: readonly AudioRow[];
  /** Subtitle tracks in file order; "Off" is not among them. */
  readonly subtitles: readonly SubtitleRow[];
  /** Whether "Off" is the subtitle choice that plays. */
  readonly subtitlesOff: boolean;
  /** The quality rows to show: the steps behind "Show all steps" are left out until asked for. */
  readonly quality: readonly QualityRow[];
  /** Whether any step is hidden or shown extra, which makes "Show all steps" meaningful. */
  readonly hasSteps: boolean;
  readonly showAllSteps: boolean;
  /** The step Automatic plays at now; `undefined` while it plays the original. */
  readonly automaticStep: QualityRow | undefined;
  readonly subtitleSize: SubtitleSize;
  /** Why the subtitle size cannot be changed now; `undefined` when it can. */
  readonly sizeLockedBy: "burnIn" | undefined;
  /** A switch is replacing the session; the panel shows a spinner (PL-11). */
  readonly switching: boolean;
  /** Whether the next queue item starts by itself (PL-14). */
  readonly autoPlay: boolean;
  /** The playback speed (PL-15). */
  readonly speed: PlaybackSpeed;
}

/** What the panel's controls do. */
export interface PlaybackOptionsActions {
  readonly toggle: () => void;
  readonly close: () => void;
  readonly chooseAudio: (streamIndex: number) => void;
  /** Chooses a subtitle track, or "off" with {@link subtitlesOffTrackId}. */
  readonly chooseSubtitle: (trackId: string) => void;
  readonly chooseQuality: (row: QualityRow) => void;
  readonly toggleAllSteps: () => void;
  readonly setSubtitleSize: (size: SubtitleSize) => void;
  readonly setAutoPlay: (enabled: boolean) => void;
  readonly setSpeed: (speed: PlaybackSpeed) => void;
}

/** The panel's state and actions. */
export interface PlaybackOptions {
  readonly view: PlaybackOptionsView;
  readonly actions: PlaybackOptionsActions;
  /** The subtitle the player draws itself, if a text track is chosen. */
  readonly sidecarTrack: SubtitleTrack | undefined;
  /** The control bar's options button; focus returns there when the panel closes. */
  readonly toggleRef: RefObject<HTMLButtonElement | null>;
}

/** The parts of the playback the panel reads and drives. */
export type PlaybackOptionsInput = Pick<
  PlaybackSessionView,
  | "session"
  | "selection"
  | "tracks"
  | "profile"
  | "switching"
  | "applySwitch"
  | "setAutomaticQuality"
>;

function choiceOfRow(row: QualityRow): QualityChoice {
  if (row.kind === "limited") {
    return { kind: "limited", maxBitRate: row.maxBitRate };
  }
  return { kind: row.kind };
}

/** Whether the server could not burn in the track the viewer asked for, and subtitles are off. */
function burnInRefused(
  session: PlaybackSession | undefined,
  requestedBurnIn: string | undefined,
  shownId: string,
): string | undefined {
  const refused = session !== undefined && sessionToasts(session).includes("burnInUnavailable");
  return refused && shownId === subtitlesOffTrackId ? requestedBurnIn : undefined;
}

/** What the panel remembers besides the playback. */
interface PanelState {
  readonly open: boolean;
  readonly showAllSteps: boolean;
  readonly storedChoice: QualityChoice;
  readonly subtitleSize: SubtitleSize;
  /** The burned-in subtitle the viewer asked for last. */
  readonly requestedBurnIn: string | undefined;
}

/** The rows and flags the panel renders. */
function buildView(
  playback: PlaybackOptionsInput,
  shownId: string,
  panel: PanelState,
): Omit<PlaybackOptionsView, "autoPlay" | "speed"> {
  const { session, selection, tracks, profile } = playback;
  const choice = availableQualityChoice(panel.storedChoice, tracks?.quality ?? []);
  const allQuality = qualityRows(tracks?.quality ?? [], choice, profile?.hls.delivery === "mse");
  return {
    open: panel.open,
    audio: audioRows(
      tracks?.audio ?? [],
      currentAudioIndex(selection, session),
      profile?.hls.audioCodecs ?? [],
    ),
    subtitles: subtitleRows(
      tracks?.subtitles ?? [],
      shownId,
      burnInRefused(session, panel.requestedBurnIn, shownId),
    ),
    subtitlesOff: shownId === subtitlesOffTrackId,
    quality: visibleQualityRows(allQuality, panel.showAllSteps),
    hasSteps: allQuality.some((row) => row.kind === "limited"),
    showAllSteps: panel.showAllSteps,
    automaticStep: activeStep(allQuality, selection),
    subtitleSize: panel.subtitleSize,
    sizeLockedBy: subtitleIsBurnedIn(session) ? "burnIn" : undefined,
    switching: playback.switching,
  };
}

/** What the actions need: the playback, its state and the setters of the panel's own state. */
interface ActionContext {
  readonly playback: PlaybackOptionsInput;
  readonly state: SwitchState | undefined;
  readonly set: {
    readonly open: (open: boolean | ((was: boolean) => boolean)) => void;
    readonly showAllSteps: (show: boolean | ((was: boolean) => boolean)) => void;
    readonly storedChoice: (choice: QualityChoice) => void;
    readonly subtitleSize: (size: SubtitleSize) => void;
    readonly requestedBurnIn: (trackId: string | undefined) => void;
  };
}

/** The actions except closing, which the hook itself owns because it moves focus. */
function createActions({
  playback,
  state,
  set,
}: ActionContext): Omit<PlaybackOptionsActions, "close" | "setAutoPlay" | "setSpeed"> {
  const subtitleTracks = playback.tracks?.subtitles ?? [];
  return {
    toggle: () => {
      set.open((wasOpen) => !wasOpen);
    },
    chooseAudio: (streamIndex) => {
      if (state !== undefined && playback.tracks !== undefined) {
        playback.applySwitch(planAudioSwitch(state, playback.tracks.audio, streamIndex));
      }
    },
    chooseSubtitle: (trackId) => {
      if (state === undefined) {
        return;
      }
      const burnsIn = subtitleTracks.some((track) => track.id === trackId && !track.sidecar);
      set.requestedBurnIn(burnsIn ? trackId : undefined);
      playback.applySwitch(planSubtitleSwitch(state, subtitleTracks, trackId));
    },
    chooseQuality: (row) => {
      const next = choiceOfRow(row);
      set.storedChoice(next);
      writeQualityChoice(browserLocalStorage(), next);
      playback.setAutomaticQuality(next.kind === "automatic");
      if (state !== undefined && next.kind !== "automatic") {
        playback.applySwitch(planQualitySwitch(state, startingMaxBitRate(next)));
      }
    },
    toggleAllSteps: () => {
      set.showAllSteps((was) => !was);
    },
    setSubtitleSize: (size) => {
      set.subtitleSize(size);
      writeSubtitleSize(browserLocalStorage(), size);
    },
  };
}

/**
 * The playback options panel's logic (PL-10 … PL-13): the rows for audio, subtitles and
 * quality as the file and the running session allow them, and the choices. Switching goes
 * through player-core's plans, so what a choice costs (a restart, or nothing for a text
 * subtitle) is decided there; the engine carries the plan out. Quality and subtitle size are
 * remembered per device.
 */
export function usePlaybackOptions(
  playback: PlaybackOptionsInput,
  preferences: PlaybackPreferences,
): PlaybackOptions {
  const { session, selection, tracks } = playback;
  const toggleRef = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  const [showAllSteps, setShowAllSteps] = useState(false);
  const [storedChoice, setStoredChoice] = useState(() => readQualityChoice(browserLocalStorage()));
  const [subtitleSize, setSubtitleSize] = useState(() => readSubtitleSize(browserLocalStorage()));
  const [requestedBurnIn, setRequestedBurnIn] = useState<string>();
  const subtitleTracks = tracks?.subtitles ?? [];
  const state: SwitchState | undefined = session === undefined ? undefined : { selection, session };
  const shownId =
    state === undefined ? subtitlesOffTrackId : shownSubtitleTrackId(state, subtitleTracks);
  const panel = { open, showAllSteps, storedChoice, subtitleSize, requestedBurnIn };
  const actions = createActions({
    playback,
    state,
    set: {
      open: setOpen,
      showAllSteps: setShowAllSteps,
      storedChoice: setStoredChoice,
      subtitleSize: setSubtitleSize,
      requestedBurnIn: setRequestedBurnIn,
    },
  });
  const close = () => {
    setOpen(false);
    // Some browsers (Safari) do not focus a button on click, so the dialog cannot rely on
    // having remembered it as the opener.
    toggleRef.current?.focus();
  };
  const { autoPlay, speed, setAutoPlay, setSpeed } = preferences;
  return {
    view: { ...buildView(playback, shownId, panel), autoPlay, speed },
    actions: { ...actions, close, setAutoPlay, setSpeed },
    sidecarTrack: subtitleTracks.find((track) => track.id === shownId && track.sidecar),
    toggleRef,
  };
}

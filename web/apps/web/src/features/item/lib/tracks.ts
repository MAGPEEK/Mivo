import type { MediaFile, MediaStream } from "@mivo/api-client";

import { audioStreamLabel, subtitleStreamLabel } from "./formatLabels";
import { streamsOfKind } from "./primaryStreams";

/** One audio or subtitle track as the track lists show it. */
export interface Track {
  /** Stream index in the container; unique within the file. */
  readonly index: number;
  /** Language tag as stored in the file, usually ISO 639-2; `undefined` when absent. */
  readonly language: string | undefined;
  /** Title as stored in the file, e.g. "Director's commentary"; `undefined` when absent. */
  readonly title: string | undefined;
  /** Format label, e.g. `AC-3 5.1` or `SRT`; empty when unknown. */
  readonly format: string;
  readonly isDefault: boolean;
  readonly isForced: boolean;
}

/** Audio tracks of a file in container order. */
export function audioTracks(file: MediaFile): Track[] {
  return streamsOfKind(file, "audio").map((stream) => toTrack(stream, audioStreamLabel(stream)));
}

/** Subtitle tracks of a file in container order. */
export function subtitleTracks(file: MediaFile): Track[] {
  return streamsOfKind(file, "subtitle").map((stream) =>
    toTrack(stream, subtitleStreamLabel(stream)),
  );
}

function toTrack(stream: MediaStream, format: string): Track {
  return {
    index: stream.index,
    language: stream.language,
    title: emptyToUndefined(stream.title),
    format,
    isDefault: stream.default,
    isForced: stream.forced,
  };
}

function emptyToUndefined(text: string | undefined): string | undefined {
  return text === "" ? undefined : text;
}

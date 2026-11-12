import type { MediaFile } from "@mivo/api-client";
import {
  audioTracks as coreAudioTracks,
  defaultAudioTrack,
  defaultSubtitleTrack,
  resolutionLabel,
  subtitlesOffTrackId,
  subtitleTracks as coreSubtitleTracks,
  type AudioTrack,
  type SubtitleTrack,
} from "@mivo/player-core";

import { audioStreamLabel } from "./formatLabels";
import { languageName } from "./languageName";
import { primaryVideoStream, streamsOfKind } from "./primaryStreams";

/** Separates the parts of an option label, e.g. `Deutsch · AC-3 5.1`. */
const labelSeparator = " · ";
const bitsPerMegabit = 1_000_000;

/** The translated words the selector labels need. */
export interface VersionTexts {
  /** The UI language, for language names and number formats. */
  readonly language: string;
  readonly unknownLanguage: string;
  /** "8.2 Mbit/s" for a bit rate given in Mbit/s. */
  readonly megabits: (value: number) => string;
  /** The flag of a forced subtitle track. */
  readonly forced: string;
  /** The note on a subtitle track that only plays burned into the video (a transcode). */
  readonly burnedIn: string;
}

/** One entry of a selector; `value` is what the choice is stored and compared by. */
export interface ChoiceOption {
  readonly value: string;
  readonly label: string;
}

/** What the selectors offer for one version (one file) of an item. */
export interface VersionChoices {
  readonly fileId: string;
  /** "Director's Cut · 1080p · 8.2 Mbit/s". */
  readonly label: string;
  /** Audio tracks in file order; the value is the stream index. */
  readonly audio: readonly ChoiceOption[];
  /** Subtitle tracks in file order, without "Off"; the value is the track id. */
  readonly subtitles: readonly ChoiceOption[];
  /** Value of the audio track that plays when nothing is chosen; `undefined` without audio. */
  readonly defaultAudio: string | undefined;
  /** Value of the subtitle track shown by default; {@link subtitlesOffTrackId} for none. */
  readonly defaultSubtitle: string;
  /** The default subtitle for each audio track (by value) the viewer may pick instead. */
  readonly defaultSubtitleByAudio: Readonly<Record<string, string>>;
}

/**
 * Whether the file is a version of its own: one that can be played and not a later part of a
 * version split over several files (the player continues from part to part by itself).
 */
export function isVersionFile(file: MediaFile): boolean {
  return file.available && (file.partNumber === undefined || file.partNumber <= 1);
}

/** The selector entries and defaults of one file. */
export function versionChoices(file: MediaFile, texts: VersionTexts): VersionChoices {
  const audio = coreAudioTracks(file);
  const subtitles = coreSubtitleTracks(file);
  return {
    fileId: file.id,
    label: versionLabel(file, texts),
    audio: audio.map((track) => ({
      value: String(track.index),
      label: audioLabel(file, track, texts),
    })),
    subtitles: subtitles.map((track) => ({ value: track.id, label: subtitleLabel(track, texts) })),
    defaultAudio: defaultAudioValue(file, audio),
    defaultSubtitle: defaultSubtitleValue(file, audio, subtitles),
    defaultSubtitleByAudio: subtitleByAudio(file, audio, subtitles),
  };
}

// The subtitle that goes with a picked audio track: the player's policy for that track's
// language. Only the server's pick for its own default audio is the server's word.
function subtitleByAudio(
  file: MediaFile,
  audio: readonly AudioTrack[],
  subtitles: readonly SubtitleTrack[],
): Readonly<Record<string, string>> {
  const defaultAudio = defaultAudioValue(file, audio);
  return Object.fromEntries(
    audio.map((track) => {
      const value = String(track.index);
      const subtitle =
        value === defaultAudio
          ? defaultSubtitleValue(file, audio, subtitles)
          : (defaultSubtitleTrack(subtitles, track.language)?.id ?? subtitlesOffTrackId);
      return [value, subtitle];
    }),
  );
}

// The file's own width and height (API 1.16) save a look at the streams; older servers omit them.
function resolutionText(file: MediaFile): string {
  const height = file.height ?? primaryVideoStream(file)?.height;
  if (height === undefined) {
    return "";
  }
  return resolutionLabel(height, file.width ?? primaryVideoStream(file)?.width);
}

function versionLabel(file: MediaFile, texts: VersionTexts): string {
  const bitRate = file.bitRate === undefined ? "" : texts.megabits(file.bitRate / bitsPerMegabit);
  const name = nonEmpty(file.versionLabel) === "" ? file.fileName : nonEmpty(file.versionLabel);
  return [name, resolutionText(file), bitRate].filter((part) => part !== "").join(labelSeparator);
}

function trackLanguage(language: string | undefined, texts: VersionTexts): string {
  return languageName(language, texts.language) ?? texts.unknownLanguage;
}

function nonEmpty(text: string | undefined): string {
  return text ?? "";
}

function audioLabel(file: MediaFile, track: AudioTrack, texts: VersionTexts): string {
  const stream = streamsOfKind(file, "audio").find((candidate) => candidate.index === track.index);
  return [
    trackLanguage(track.language, texts),
    nonEmpty(track.title),
    stream === undefined ? "" : audioStreamLabel(stream),
  ]
    .filter((part) => part !== "")
    .join(labelSeparator);
}

function subtitleLabel(track: SubtitleTrack, texts: VersionTexts): string {
  return [
    trackLanguage(track.language, texts),
    track.sdh ? "SDH" : "",
    nonEmpty(track.title),
    track.format.toUpperCase(),
    track.forced ? texts.forced : "",
    // Only a text track with a WebVTT rendition plays without a transcode.
    track.sidecar ? "" : texts.burnedIn,
  ]
    .filter((part) => part !== "")
    .join(labelSeparator);
}

// The server's pick wins (it knows the viewer's profile); a server that reports none leaves it
// to the same policy the player preselects with.
function defaultAudioValue(file: MediaFile, audio: readonly AudioTrack[]): string | undefined {
  const reported = file.defaultAudioStreamIndex;
  if (
    reported !== undefined &&
    reported !== null &&
    audio.some((track) => track.index === reported)
  ) {
    return String(reported);
  }
  const fallback = defaultAudioTrack(audio);
  return fallback === undefined ? undefined : String(fallback.index);
}

function defaultSubtitleValue(
  file: MediaFile,
  audio: readonly AudioTrack[],
  subtitles: readonly SubtitleTrack[],
): string {
  const reported = file.defaultSubtitleTrackId;
  if (reported === null) {
    return subtitlesOffTrackId;
  }
  if (reported !== undefined) {
    return subtitles.some((track) => track.id === reported) ? reported : subtitlesOffTrackId;
  }
  const audioIndex = defaultAudioValue(file, audio);
  const audioLanguage = audio.find((track) => String(track.index) === audioIndex)?.language;
  return defaultSubtitleTrack(subtitles, audioLanguage)?.id ?? subtitlesOffTrackId;
}

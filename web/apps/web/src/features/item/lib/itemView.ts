import type { ItemDetail, MediaFile } from "@mivo/api-client";
import { resolutionLabel } from "@mivo/player-core";

import { durationParts, type DurationParts } from "./duration";
import { formatFileSize } from "./fileSize";
import { languageName } from "./languageName";
import { playTarget, type UnavailableReason } from "./playTarget";
import { primaryVideoStream } from "./primaryStreams";
import { technicalChips } from "./technicalChips";
import { audioTracks, subtitleTracks, type Track } from "./tracks";

/** A track with its language name in the UI language. */
export interface TrackView extends Track {
  /** Language name, or the translated "unknown language". */
  readonly languageName: string;
}

/** The primary file's name and size, ready to show. */
export interface FileView {
  readonly name: string;
  readonly size: string;
}

/** What the Play button shows: a link to the player, or the reason it is disabled. */
export type PlayView =
  | { readonly kind: "playable"; readonly fileId: string }
  | { readonly kind: "unavailable"; readonly explanation: string };

/** Everything the detail page shows of a loaded item, as display text. */
export interface ItemView {
  readonly title: string;
  /** First letter of the title for the poster placeholder; artwork arrives in milestone 3. */
  readonly initial: string;
  /** Short facts below the title, e.g. `["1 h 30 min", "1080p"]`. */
  readonly facts: readonly string[];
  /** Technical chips, e.g. `["MKV", "HEVC 10-bit HDR10", "AC-3 5.1", "SRT"]`. */
  readonly chips: readonly string[];
  /** `undefined` when the item has no file. */
  readonly file: FileView | undefined;
  readonly audioTracks: readonly TrackView[];
  readonly subtitleTracks: readonly TrackView[];
  readonly play: PlayView;
}

/** The translated texts the view needs; the hook builds them from i18next. */
export interface ItemViewTexts {
  /** The UI language, for language names and number formats, e.g. `de`. */
  readonly language: string;
  readonly duration: (parts: DurationParts) => string;
  readonly unknownLanguage: string;
  readonly unavailable: (reason: UnavailableReason) => string;
}

/** The parts of an {@link ItemView} that describe the primary file. */
type FileParts = Pick<ItemView, "facts" | "chips" | "file" | "audioTracks" | "subtitleTracks">;

const noFileParts: FileParts = {
  facts: [],
  chips: [],
  file: undefined,
  audioTracks: [],
  subtitleTracks: [],
};

/**
 * Turns an item into what the detail page shows. Everything is derived from the primary
 * (first) file, which is also the one Play opens (API contract: `ItemDetail.files`).
 */
export function toItemView(item: ItemDetail, texts: ItemViewTexts): ItemView {
  const [primaryFile] = item.files;
  return {
    title: item.title,
    initial: titleInitial(item.title),
    play: playView(item, texts),
    ...(primaryFile === undefined ? noFileParts : fileParts(primaryFile, texts)),
  };
}

function fileParts(file: MediaFile, texts: ItemViewTexts): FileParts {
  const toTrackView = (track: Track): TrackView => ({
    ...track,
    languageName: languageName(track.language, texts.language) ?? texts.unknownLanguage,
  });
  return {
    facts: facts(file, texts),
    chips: technicalChips(file),
    file: { name: file.fileName, size: formatFileSize(file.sizeBytes, texts.language) },
    audioTracks: audioTracks(file).map(toTrackView),
    subtitleTracks: subtitleTracks(file).map(toTrackView),
  };
}

function titleInitial(title: string): string {
  // Array.from splits by code point, so a title starting with an emoji keeps it whole.
  const [first = ""] = Array.from(title.trim());
  return first.toLocaleUpperCase();
}

function facts(file: MediaFile, texts: ItemViewTexts): string[] {
  const video = primaryVideoStream(file);
  const resolution = video?.height === undefined ? "" : resolutionLabel(video.height, video.width);
  const duration =
    file.durationSeconds === undefined ? "" : texts.duration(durationParts(file.durationSeconds));
  return [duration, resolution].filter((fact) => fact !== "");
}

function playView(item: ItemDetail, texts: ItemViewTexts): PlayView {
  const target = playTarget(item);
  return target.kind === "playable"
    ? target
    : { kind: "unavailable", explanation: texts.unavailable(target.reason) };
}

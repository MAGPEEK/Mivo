import type {
  ImageRef,
  ItemAncestor,
  ItemDetail,
  ItemKind,
  MediaFile,
  PersonRole,
} from "@mivo/api-client";
import { bookTimeline, resolutionLabel } from "@mivo/player-core";

import { formatAirDate } from "./airDate";
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

/** An item above the shown one, for the links back to its show and season. */
export type AncestorView = Pick<ItemAncestor, "id" | "kind" | "title" | "number">;

/** Why Play is disabled: a file problem, or a show or season without an episode to start. */
export type PlayUnavailableReason = UnavailableReason | "noEpisodes";

/**
 * What the Play button shows: a link to the player, or the reason it is disabled. A movie or
 * episode names its file; a show or season only the episode to start (the player then picks
 * that episode's primary file).
 */
export type PlayView =
  | { readonly kind: "playable"; readonly itemId: string; readonly fileId?: string }
  | { readonly kind: "unavailable"; readonly explanation: string };

/** Everything the detail page shows of a loaded item, as display text. */
export interface ItemView {
  readonly kind: ItemKind;
  readonly title: string;
  /** Release or first-air year; undefined when unknown. */
  readonly year: string | undefined;
  readonly summary: string | undefined;
  readonly genres: readonly string[];
  /** Air date of an episode, localised; undefined when unknown. */
  readonly airDate: string | undefined;
  /** Show first, then season; empty for top-level items. */
  readonly ancestors: readonly AncestorView[];
  readonly previousId: string | undefined;
  readonly nextId: string | undefined;
  /** The poster image; `undefined` shows the placeholder. */
  readonly poster: ImageRef | undefined;
  /** The 16:9 still of an episode or extra; `undefined` shows the placeholder. */
  readonly still: ImageRef | undefined;
  /** The 1:1 cover of an album or image of an artist; `undefined` shows the placeholder. */
  readonly square: ImageRef | undefined;
  /** Short facts below the title, e.g. `["1 h 30 min", "1080p"]`. */
  readonly facts: readonly string[];
  /** Technical chips, e.g. `["MKV", "HEVC 10-bit HDR10", "AC-3 5.1", "SRT"]`. */
  readonly chips: readonly string[];
  /** `undefined` when the item has no file. */
  readonly file: FileView | undefined;
  readonly audioTracks: readonly TrackView[];
  readonly subtitleTracks: readonly TrackView[];
  readonly play: PlayView;
  /** Where the metadata came from, e.g. `["nfo", "tmdb"]`; empty when unknown. */
  readonly metadataSources: readonly string[];
}

/** The translated texts the view needs; the hook builds them from i18next. */
export interface ItemViewTexts {
  /** The UI language, for language names and number formats, e.g. `de`. */
  readonly language: string;
  readonly duration: (parts: DurationParts) => string;
  readonly unknownLanguage: string;
  readonly unavailable: (reason: PlayUnavailableReason) => string;
  /** "5 seasons", with the plural form of the language. */
  readonly seasons: (count: number) => string;
  /** "62 episodes", with the plural form of the language. */
  readonly episodes: (count: number) => string;
  /** "12 tracks", with the plural form of the language. */
  readonly trackCount: (count: number) => string;
  /** "By Ada Inkwell" for the authors of an audiobook, already joined. */
  readonly authors: (names: string) => string;
  /** "Narrated by Nora Vale" for the narrators of an audiobook, already joined. */
  readonly narrators: (names: string) => string;
  /** "3 chapters", with the plural form of the language. */
  readonly chapterCount: (count: number) => string;
  /** "24 photos" in a photo album and its sub-albums, with the plural form of the language. */
  readonly photoCount: (count: number) => string;
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
  const fileView = primaryFile === undefined ? noFileParts : fileParts(primaryFile, texts);
  return {
    kind: item.kind,
    title: item.title,
    year: item.year === undefined ? undefined : String(item.year),
    summary: item.summary,
    genres: item.genres ?? [],
    airDate: formatAirDate(item.releaseDate, texts.language),
    ancestors: (item.ancestors ?? []).map(toAncestorView),
    previousId: item.previousId,
    nextId: item.nextId,
    poster: item.artwork?.poster,
    still: item.artwork?.thumb,
    square: item.artwork?.square,
    play: playView(item, texts),
    metadataSources: item.metadataSources ?? [],
    ...fileView,
    facts:
      item.kind === "audiobook"
        ? audiobookFacts(item, texts)
        : [...itemFacts(item, texts), ...fileView.facts],
  };
}

function namesWithRole(item: ItemDetail, role: PersonRole, language: string): string {
  const names = (item.people ?? []).filter((person) => person.role === role);
  return new Intl.ListFormat(language, { style: "long", type: "conjunction" }).format(
    names.map((person) => person.name),
  );
}

// An audiobook names its authors and narrators, and its length is that of all its parts: the
// primary file is only the first part of a book of several.
function audiobookFacts(item: ItemDetail, texts: ItemViewTexts): string[] {
  const authors = namesWithRole(item, "author", texts.language);
  const narrators = namesWithRole(item, "narrator", texts.language);
  const chapters = item.chapters ?? [];
  const { totalSeconds } = bookTimeline(item.files, chapters);
  return [
    authors === "" ? "" : texts.authors(authors),
    narrators === "" ? "" : texts.narrators(narrators),
    item.year === undefined ? "" : String(item.year),
    totalSeconds > 0 ? texts.duration(durationParts(totalSeconds)) : "",
    chapters.length > 0 ? texts.chapterCount(chapters.length) : "",
  ].filter((fact) => fact !== "");
}

function toAncestorView(ancestor: ItemAncestor): AncestorView {
  return ancestor;
}

// Year and counts of the item itself, before the facts of its file.
function itemFacts(item: ItemDetail, texts: ItemViewTexts): string[] {
  const facts = item.year === undefined ? [] : [String(item.year)];
  const isShow = item.kind === "show";
  if (isShow && item.childCount !== undefined) {
    facts.push(texts.seasons(item.childCount));
  }
  if (isShow && item.leafCount !== undefined) {
    facts.push(texts.episodes(item.leafCount));
  }
  if (item.kind === "season" && item.childCount !== undefined) {
    facts.push(texts.episodes(item.childCount));
  }
  if (item.kind === "album" && item.childCount !== undefined) {
    facts.push(texts.trackCount(item.childCount));
  }
  if (item.kind === "photo_album" && item.leafCount !== undefined) {
    facts.push(texts.photoCount(item.leafCount));
  }
  return facts;
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

function facts(file: MediaFile, texts: ItemViewTexts): string[] {
  const video = primaryVideoStream(file);
  const resolution = video?.height === undefined ? "" : resolutionLabel(video.height, video.width);
  const duration =
    file.durationSeconds === undefined ? "" : texts.duration(durationParts(file.durationSeconds));
  return [duration, resolution].filter((fact) => fact !== "");
}

function playView(item: ItemDetail, texts: ItemViewTexts): PlayView {
  if (item.kind === "show" || item.kind === "season") {
    return item.playItemId === undefined
      ? { kind: "unavailable", explanation: texts.unavailable("noEpisodes") }
      : { kind: "playable", itemId: item.playItemId };
  }
  // A still photo has an image file but nothing to play; only a video names its play item.
  if (item.kind === "photo" && item.playItemId === undefined) {
    return { kind: "unavailable", explanation: texts.unavailable("noFile") };
  }
  const target = playTarget(item);
  return target.kind === "playable"
    ? { kind: "playable", itemId: item.id, fileId: target.fileId }
    : { kind: "unavailable", explanation: texts.unavailable(target.reason) };
}

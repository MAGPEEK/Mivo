import type {
  ImageRef,
  ItemAncestor,
  ItemDetail,
  ItemKind,
  MediaFile,
  PersonRole,
  ViewerState,
} from "@mivo/api-client";
import { bookTimeline, resolutionLabel } from "@mivo/player-core";

import { resumeSeconds } from "../../../shared/lib/watchState";
import { formatDuration } from "../../library";
import { formatAirDate } from "./airDate";
import type { DurationParts } from "./duration";
import { formatFileSize } from "./fileSize";
import { languageName } from "./languageName";
import { playTarget, type UnavailableReason } from "./playTarget";
import { primaryVideoStream } from "./primaryStreams";
import { runtimeLabel } from "./runtimeLabel";
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

/** Where Play continues (DT-05): the stored position of a movie or episode in progress. */
export interface ResumeView {
  /** Whole seconds, for the player link. */
  readonly seconds: number;
  /** Clock notation for the button, e.g. `42:10` or `1:02:10`. */
  readonly position: string;
}

/**
 * What the Play button shows: a link to the player, or the reason it is disabled. A movie or
 * episode names its file; a show or season only the episode to start (the player then picks
 * that episode's primary file). With `resume` the button continues there and offers "Play
 * from start".
 */
export type PlayView =
  | {
      readonly kind: "playable";
      readonly itemId: string;
      readonly fileId?: string;
      readonly resume?: ResumeView;
    }
  | { readonly kind: "unavailable"; readonly explanation: string };

/** Everything the detail page shows of a loaded item, as display text. */
export interface ItemView {
  readonly kind: ItemKind;
  /** The library the item belongs to, for the genre links to its Browse pivot. */
  readonly libraryId: string;
  readonly title: string;
  /** Title in the original language; undefined when unknown or equal to the title. */
  readonly originalTitle: string | undefined;
  /** Age rating as stored, e.g. `FSK 12`; undefined when unknown. */
  readonly contentRating: string | undefined;
  /** Season, episode or track number; undefined when unknown or for other kinds. */
  readonly number: number | undefined;
  /** Last episode number of a file holding several episodes. */
  readonly numberEnd: number | undefined;
  /** Release or first-air year; undefined when unknown. */
  readonly year: string | undefined;
  readonly summary: string | undefined;
  /** Tagline of a movie or show, shown above the summary. */
  readonly tagline: string | undefined;
  readonly genres: readonly string[];
  /** "Director: Mara Lindqvist", already translated; undefined without a credited director. */
  readonly directorLine: string | undefined;
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
  /** The wide backdrop behind the header; `undefined` falls back to the item's artwork. */
  readonly backdrop: ImageRef | undefined;
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
  /** The profile's watch state; `undefined` without any, and for kinds that have none. */
  readonly viewerState: ViewerState | undefined;
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
  /** "Director: Mara Lindqvist" for the directors, already joined; `count` picks the plural. */
  readonly directors: (names: string, count: number) => string;
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
    libraryId: item.libraryId,
    title: item.title,
    originalTitle: originalTitle(item),
    contentRating: textOrUndefined(item.contentRating),
    number: item.number,
    numberEnd: item.numberEnd,
    year: item.year === undefined ? undefined : String(item.year),
    summary: textOrUndefined(item.summary),
    tagline: textOrUndefined(item.tagline),
    genres: (item.genres ?? []).filter((genre) => genre.trim() !== ""),
    directorLine: directorLine(item, texts),
    airDate: formatAirDate(item.releaseDate, texts.language),
    ancestors: (item.ancestors ?? []).map(toAncestorView),
    previousId: item.previousId,
    nextId: item.nextId,
    poster: item.artwork?.poster,
    still: item.artwork?.thumb,
    square: item.artwork?.square,
    backdrop: item.artwork?.backdrop,
    play: playView(item, texts),
    metadataSources: item.metadataSources ?? [],
    viewerState: item.viewerState,
    ...fileView,
    facts:
      item.kind === "audiobook"
        ? audiobookFacts(item, texts)
        : [...itemFacts(item, texts), ...fileView.facts],
  };
}

// Blank metadata counts as missing, so no line or separator shows for it.
function textOrUndefined(text: string | undefined): string | undefined {
  return text === undefined || text.trim() === "" ? undefined : text;
}

// The original title only adds information when it differs from the shown one.
function originalTitle(item: ItemDetail): string | undefined {
  const original = textOrUndefined(item.originalTitle);
  return original?.trim().toLocaleLowerCase() === item.title.trim().toLocaleLowerCase()
    ? undefined
    : original;
}

function peopleWithRole(item: ItemDetail, role: PersonRole): string[] {
  return (item.people ?? [])
    .filter((person) => person.role === role && person.name.trim() !== "")
    .map((person) => person.name);
}

function joinNames(names: readonly string[], language: string): string {
  return new Intl.ListFormat(language, { style: "long", type: "conjunction" }).format(names);
}

function namesWithRole(item: ItemDetail, role: PersonRole, language: string): string {
  return joinNames(peopleWithRole(item, role), language);
}

function directorLine(item: ItemDetail, texts: ItemViewTexts): string | undefined {
  const names = peopleWithRole(item, "director");
  return names.length === 0
    ? undefined
    : texts.directors(joinNames(names, texts.language), names.length);
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
    runtimeLabel(totalSeconds, texts.duration) ?? "",
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
  const duration = runtimeLabel(file.durationSeconds, texts.duration) ?? "";
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
  if (target.kind !== "playable") {
    return { kind: "unavailable", explanation: texts.unavailable(target.reason) };
  }
  const resume = resumeView(item.viewerState);
  return {
    kind: "playable",
    itemId: item.id,
    fileId: target.fileId,
    ...(resume === undefined ? {} : { resume }),
  };
}

function resumeView(viewerState: ViewerState | undefined): ResumeView | undefined {
  const position = resumeSeconds(viewerState);
  if (position === undefined) {
    return undefined;
  }
  const seconds = Math.floor(position);
  // Less than a second played is no place to resume from.
  return seconds > 0 ? { seconds, position: formatDuration(seconds) } : undefined;
}

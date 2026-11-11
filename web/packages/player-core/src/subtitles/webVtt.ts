// Sidecar subtitles (ADR 0030 §3): the server delivers every text track as WebVTT keyed by
// file, so cue times are file positions. The player parses the cues, maps the media element's
// clock to the file position and renders the active cues itself; no DOM is touched here.

import { mediaClockOffset, type TimelineSession } from "../timeline/timeline";

/** One subtitle cue; times are file positions in seconds. */
export interface SubtitleCue {
  readonly start: number;
  readonly end: number;
  /** Plain text without markup; lines are separated by `\n`. */
  readonly text: string;
}

/**
 * Most cues one track may hold. A feature film has a few thousand; the bound keeps a broken
 * or hostile file from exhausting memory.
 */
export const maxSubtitleCues = 20_000;

const byteOrderMark = "﻿";
const timingArrow = "-->";
const secondsPerMinute = 60;
const secondsPerHour = 3600;
const millisecondsPerSecond = 1000;
const hexadecimalRadix = 16;
const decimalRadix = 10;

// "mm:ss.ttt" or "hh:mm:ss.ttt"; a comma is tolerated because converted SubRip files carry it.
const timestampPattern = /^(?:(\d+):)?(\d{1,2}):(\d{2})[.,](\d{3})$/;
const lineBreakPattern = /\r\n|\r|\n/;
const whitespacePattern = /\s+/;
// Cue markup: <b>, <i>, <c.class>, <v Speaker>, <00:01:02.000> karaoke timestamps, …
const tagPattern = /<[^>]*>/g;
const entityPattern = /&(#x[0-9a-f]{1,6}|#\d{1,7}|[a-z]{2,4});/gi;

const namedEntities: ReadonlyMap<string, string> = new Map([
  ["amp", "&"],
  ["lt", "<"],
  ["gt", ">"],
  ["quot", '"'],
  ["nbsp", " "],
  ["lrm", "‎"],
  ["rlm", "‏"],
]);

const highestCodePoint = 0x10ffff;

function parseTimestamp(text: string): number | undefined {
  const match = timestampPattern.exec(text);
  if (match === null) {
    return undefined;
  }
  const [, hours = "0", minutes = "0", seconds = "0", milliseconds = "0"] = match;
  return (
    Number(hours) * secondsPerHour +
    Number(minutes) * secondsPerMinute +
    Number(seconds) +
    Number(milliseconds) / millisecondsPerSecond
  );
}

function decodeEntity(entity: string, name: string): string {
  if (!name.startsWith("#")) {
    return namedEntities.get(name.toLowerCase()) ?? entity;
  }
  const isHexadecimal = name[1] === "x" || name[1] === "X";
  const codePoint = isHexadecimal
    ? Number.parseInt(name.slice(2), hexadecimalRadix)
    : Number.parseInt(name.slice(1), decimalRadix);
  return codePoint > 0 && codePoint <= highestCodePoint ? String.fromCodePoint(codePoint) : entity;
}

/** Drops markup first, so an escaped `&lt;b&gt;` stays visible text. */
function plainText(lines: readonly string[]): string {
  return lines
    .map((line) => line.replace(tagPattern, "").replace(entityPattern, decodeEntity).trim())
    .filter((line) => line !== "")
    .join("\n");
}

/** Reads `start --> end` (cue settings after it are ignored); `undefined` for any other line. */
function parseTiming(line: string): { start: number; end: number } | undefined {
  const [startText = "", arrow, endText = ""] = line.trim().split(whitespacePattern);
  const start = parseTimestamp(startText);
  const end = parseTimestamp(endText);
  if (arrow !== timingArrow || start === undefined || end === undefined || end <= start) {
    return undefined;
  }
  return { start, end };
}

/** A block is the lines between blank lines: the header, a note, a style or a cue. */
function parseBlock(block: readonly string[]): SubtitleCue | undefined {
  // The timing line is the first line, or the second one after a cue identifier.
  const timingIndex = block.findIndex((line) => line.includes(timingArrow));
  const timingLine = block[timingIndex];
  if (timingLine === undefined || timingIndex > 1) {
    return undefined;
  }
  const timing = parseTiming(timingLine);
  const text = plainText(block.slice(timingIndex + 1));
  return timing === undefined || text === "" ? undefined : { ...timing, text };
}

function blocksOf(text: string): string[][] {
  const blocks: string[][] = [];
  let current: string[] = [];
  for (const line of text.split(lineBreakPattern)) {
    if (line.trim() === "") {
      blocks.push(current);
      current = [];
    } else {
      current.push(line);
    }
  }
  blocks.push(current);
  return blocks.filter((block) => block.length > 0);
}

/**
 * Parses a WebVTT file into cues ordered by start time. Cue identifiers, settings (line,
 * position, align), markup, notes, styles and regions are ignored; the text is plain. Blocks
 * that are no valid cue are skipped, so a damaged file shows what can be read. At most
 * {@link maxSubtitleCues} cues are returned.
 */
export function parseWebVtt(text: string): SubtitleCue[] {
  const content = text.startsWith(byteOrderMark) ? text.slice(byteOrderMark.length) : text;
  const cues: SubtitleCue[] = [];
  for (const block of blocksOf(content)) {
    const cue = parseBlock(block);
    if (cue !== undefined) {
      cues.push(cue);
    }
    if (cues.length === maxSubtitleCues) {
      break;
    }
  }
  return cues.sort((first, second) => first.start - second.start);
}

/** The cues to show at file position `fileTime`: started and not yet ended, in start order. */
export function activeCues(cues: readonly SubtitleCue[], fileTime: number): SubtitleCue[] {
  return cues.filter((cue) => cue.start <= fileTime && fileTime < cue.end);
}

/**
 * The file position cues are matched against, for the media element's `currentTime`: the
 * session's actual start plus the element's clock for a stream whose timestamps begin at 0,
 * the clock itself for a progressive file. Unlike the displayed position it is not clamped to
 * the probed duration, so the last cue of a file with an imprecise duration still shows.
 */
export function subtitleFileTime(session: TimelineSession, currentTime: number): number {
  const elementTime = Number.isNaN(currentTime) || currentTime < 0 ? 0 : currentTime;
  return mediaClockOffset(session) + elementTime;
}

import { describe, expect, it } from "vitest";

import type { TimelineSession } from "../timeline/timeline";
import {
  activeCues,
  maxSubtitleCues,
  parseWebVtt,
  subtitleFileTime,
  type SubtitleCue,
} from "./webVtt";

function vtt(...blocks: string[]): string {
  return ["WEBVTT", ...blocks].join("\n\n");
}

describe("parseWebVtt", () => {
  it.each<{ name: string; text: string; expected: SubtitleCue[] }>([
    { name: "an empty file", text: "", expected: [] },
    { name: "a header only", text: "WEBVTT\n", expected: [] },
    {
      name: "a cue with hours",
      text: vtt("01:02:03.500 --> 01:02:05.000\nHello"),
      expected: [{ start: 3723.5, end: 3725, text: "Hello" }],
    },
    {
      name: "a cue without hours",
      text: vtt("00:01.000 --> 00:02.250\nHello"),
      expected: [{ start: 1, end: 2.25, text: "Hello" }],
    },
    {
      name: "a cue identifier",
      text: vtt("intro\n00:01.000 --> 00:02.000\nHello"),
      expected: [{ start: 1, end: 2, text: "Hello" }],
    },
    {
      name: "cue settings are ignored",
      text: vtt("00:01.000 --> 00:02.000 line:0 position:20% align:start\nHello"),
      expected: [{ start: 1, end: 2, text: "Hello" }],
    },
    {
      name: "several lines",
      text: vtt("00:01.000 --> 00:02.000\n- Hello\n- Hi"),
      expected: [{ start: 1, end: 2, text: "- Hello\n- Hi" }],
    },
    {
      name: "markup is dropped",
      text: vtt("00:01.000 --> 00:02.000\n<v Anna><i>Hello</i> <c.loud>you</c> <00:01.500>there"),
      expected: [{ start: 1, end: 2, text: "Hello you there" }],
    },
    {
      name: "entities are decoded",
      text: vtt(
        "00:01.000 --> 00:02.000\nA &amp; B &lt;b&gt; &quot;x&quot;&nbsp;&#233;&#xE9;&#XE9; &bogus; &#0; &#x110000;",
      ),
      expected: [{ start: 1, end: 2, text: 'A & B <b> "x" ééé &bogus; &#0; &#x110000;' }],
    },
    {
      name: "direction marks are kept",
      text: vtt("00:01.000 --> 00:02.000\n&lrm;a&rlm;"),
      expected: [{ start: 1, end: 2, text: "‎a‏" }],
    },
    {
      name: "a byte order mark and Windows line ends",
      text: "﻿WEBVTT\r\n\r\n00:01.000 --> 00:02.000\r\nHello\r\n\r\n00:03.000 --> 00:04.000\r\nBye\r\n",
      expected: [
        { start: 1, end: 2, text: "Hello" },
        { start: 3, end: 4, text: "Bye" },
      ],
    },
    {
      name: "old Mac line ends",
      text: "WEBVTT\r\r00:01.000 --> 00:02.000\rHello",
      expected: [{ start: 1, end: 2, text: "Hello" }],
    },
    {
      name: "notes, styles and regions are skipped",
      text: vtt(
        "NOTE a comment\nover two lines",
        "STYLE\n::cue { color: red }",
        "REGION\nid:top",
        "00:01.000 --> 00:02.000\nHello",
      ),
      expected: [{ start: 1, end: 2, text: "Hello" }],
    },
    {
      name: "a comma as decimal separator",
      text: vtt("00:00:01,000 --> 00:00:02,000\nHello"),
      expected: [{ start: 1, end: 2, text: "Hello" }],
    },
    {
      name: "cues are ordered by start",
      text: vtt("00:05.000 --> 00:06.000\nLater", "00:01.000 --> 00:02.000\nEarlier"),
      expected: [
        { start: 1, end: 2, text: "Earlier" },
        { start: 5, end: 6, text: "Later" },
      ],
    },
    {
      name: "damaged cues are skipped",
      text: vtt(
        "00:01.000 --> nonsense\nBad end",
        "1:2 --> 00:02.000\nBad start",
        "00:05.000 --> 00:04.000\nEnds before it starts",
        "00:06.000 -> 00:07.000\nNo arrow",
        "00:06.000 x --> 00:07.000\nArrow in the wrong place",
        "id\nmore\n00:08.000 --> 00:09.000\nTiming too late",
        "00:10.000 --> 00:11.000",
        "00:12.000 --> 00:13.000\n<i></i>",
        "00:14.000 --> 00:15.000\nGood",
      ),
      expected: [{ start: 14, end: 15, text: "Good" }],
    },
  ])("$name", ({ text, expected }) => {
    expect(parseWebVtt(text)).toEqual(expected);
  });

  it("stops at the cue limit", () => {
    const cue = "00:01.000 --> 00:02.000\nx";
    const text = vtt(...Array.from({ length: maxSubtitleCues + 5 }, () => cue));
    expect(parseWebVtt(text)).toHaveLength(maxSubtitleCues);
  });
});

describe("activeCues", () => {
  const first: SubtitleCue = { start: 1, end: 3, text: "first" };
  const overlapping: SubtitleCue = { start: 2, end: 4, text: "overlapping" };
  const last: SubtitleCue = { start: 10, end: 12, text: "last" };
  const cues = [first, overlapping, last];

  it.each<{ time: number; expected: SubtitleCue[] }>([
    { time: 0, expected: [] },
    { time: 1, expected: [first] },
    { time: 2.5, expected: [first, overlapping] },
    { time: 3, expected: [overlapping] },
    { time: 4, expected: [] },
    { time: 11.999, expected: [last] },
    { time: 12, expected: [] },
    { time: Number.NaN, expected: [] },
  ])("at $time s", ({ time, expected }) => {
    expect(activeCues(cues, time)).toEqual(expected);
  });
});

describe("subtitleFileTime", () => {
  const hls: TimelineSession = { delivery: "hls", startPositionSeconds: 600, durationSeconds: 700 };
  const file: TimelineSession = { delivery: "file", startPositionSeconds: 0, durationSeconds: 700 };

  it.each<{ name: string; session: TimelineSession; currentTime: number; expected: number }>([
    { name: "a stream adds its actual start", session: hls, currentTime: 12.5, expected: 612.5 },
    { name: "a file uses the element clock", session: file, currentTime: 12.5, expected: 12.5 },
    { name: "an unknown time is the start", session: hls, currentTime: Number.NaN, expected: 600 },
    { name: "a negative time is the start", session: hls, currentTime: -1, expected: 600 },
    {
      name: "past the probed duration is not clamped",
      session: hls,
      currentTime: 150,
      expected: 750,
    },
  ])("$name", ({ session, currentTime, expected }) => {
    expect(subtitleFileTime(session, currentTime)).toBe(expected);
  });
});

import { describe, expect, it } from "vitest";

import { bookTimeline, filePositionAt, type BookChapter, type BookPart } from "./bookTimeline";

// The single-file M4B of the corpus: three chapters of 2 s in one file of 6 s.
const m4b: BookPart = { id: "m4b", durationSeconds: 6 };
const m4bChapters: readonly BookChapter[] = [
  { title: "The Seed Vault", startMs: 0, endMs: 2_000, fileId: "m4b" },
  { title: "Brass Branches", startMs: 2_000, endMs: 4_000, fileId: "m4b" },
  { title: "Harvest Moon", startMs: 4_000, endMs: 6_000, fileId: "m4b" },
];

// A book of three MP3 parts, one chapter each; the files list them in a different order than
// the chapters play them.
const storm: BookPart = { id: "storm", durationSeconds: 3.5 };
const keeper: BookPart = { id: "keeper", durationSeconds: 3 };
const dawn: BookPart = { id: "dawn", durationSeconds: 2.25 };
const partChapters: readonly BookChapter[] = [
  { title: "Storm", startMs: 0, endMs: 3_500, fileId: "storm" },
  { title: "Keeper", startMs: 0, endMs: 3_000, fileId: "keeper" },
  { title: "Dawn", startMs: 0, endMs: 2_250, fileId: "dawn" },
];

describe("bookTimeline", () => {
  it("keeps the file offsets of a single-file book as its book offsets", () => {
    const timeline = bookTimeline([m4b], m4bChapters);

    expect(timeline.totalSeconds).toBe(6);
    expect(timeline.chapters).toEqual([
      {
        title: "The Seed Vault",
        fileId: "m4b",
        fileStartSeconds: 0,
        bookStartSeconds: 0,
        durationSeconds: 2,
      },
      {
        title: "Brass Branches",
        fileId: "m4b",
        fileStartSeconds: 2,
        bookStartSeconds: 2,
        durationSeconds: 2,
      },
      {
        title: "Harvest Moon",
        fileId: "m4b",
        fileStartSeconds: 4,
        bookStartSeconds: 4,
        durationSeconds: 2,
      },
    ]);
  });

  it("starts each part of a multi-file book where the previous part ends, in chapter order", () => {
    const timeline = bookTimeline([dawn, keeper, storm], partChapters);

    expect(
      timeline.chapters.map(({ fileId, fileStartSeconds, bookStartSeconds }) => ({
        fileId,
        fileStartSeconds,
        bookStartSeconds,
      })),
    ).toEqual([
      { fileId: "storm", fileStartSeconds: 0, bookStartSeconds: 0 },
      { fileId: "keeper", fileStartSeconds: 0, bookStartSeconds: 3.5 },
      { fileId: "dawn", fileStartSeconds: 0, bookStartSeconds: 6.5 },
    ]);
    expect(timeline.totalSeconds).toBe(8.75);
  });

  it("maps a chapter in a later part to that part and its offset inside it", () => {
    const chapters: readonly BookChapter[] = [
      { title: "One", startMs: 0, endMs: 3_000, fileId: "storm" },
      { title: "Two", startMs: 1_500, endMs: 3_000, fileId: "keeper" },
    ];

    const [, second] = bookTimeline([storm, keeper], chapters).chapters;

    expect(second).toEqual({
      title: "Two",
      fileId: "keeper",
      fileStartSeconds: 1.5,
      bookStartSeconds: 5,
      durationSeconds: 1.5,
    });
  });

  it("takes the last chapter end as the length of a part without a known duration", () => {
    const unknown: BookPart = { id: "storm" };
    const longerChapters: readonly BookChapter[] = [
      { title: "Storm", startMs: 0, endMs: 4_000, fileId: "storm" },
      { title: "Keeper", startMs: 0, endMs: 3_000, fileId: "keeper" },
    ];

    const timeline = bookTimeline([unknown, { ...keeper, durationSeconds: 2 }], longerChapters);

    expect(timeline.chapters[1]?.bookStartSeconds).toBe(4);
    expect(timeline.totalSeconds).toBe(7);
  });

  it("counts parts without chapters after the chapters' parts", () => {
    const stormOnly = partChapters.filter((chapter) => chapter.fileId === "storm");

    const timeline = bookTimeline([dawn, storm], stormOnly);

    expect(timeline.totalSeconds).toBe(5.75);
  });

  it("is empty for a book without parts or chapters", () => {
    expect(bookTimeline([], [])).toEqual({ chapters: [], totalSeconds: 0 });
  });
});

describe("filePositionAt", () => {
  const timeline = bookTimeline([storm, keeper, dawn], partChapters);

  it.each([
    { bookSeconds: 0, expected: { fileId: "storm", seconds: 0 } },
    { bookSeconds: 2, expected: { fileId: "storm", seconds: 2 } },
    { bookSeconds: 3.5, expected: { fileId: "keeper", seconds: 0 } },
    { bookSeconds: 5, expected: { fileId: "keeper", seconds: 1.5 } },
    { bookSeconds: 7, expected: { fileId: "dawn", seconds: 0.5 } },
    { bookSeconds: -1, expected: { fileId: "storm", seconds: 0 } },
    { bookSeconds: 100, expected: { fileId: "dawn", seconds: 2.25 } },
  ])("maps $bookSeconds s of the book to $expected", ({ bookSeconds, expected }) => {
    expect(filePositionAt(timeline, bookSeconds)).toEqual(expected);
  });

  it("maps into a file whose chapter starts inside it", () => {
    const single = bookTimeline([m4b], m4bChapters);

    expect(filePositionAt(single, 4.5)).toEqual({ fileId: "m4b", seconds: 4.5 });
  });

  it("is undefined for a book without chapters", () => {
    expect(filePositionAt(bookTimeline([storm], []), 1)).toBeUndefined();
  });
});

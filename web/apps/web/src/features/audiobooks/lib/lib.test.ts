import type { ItemDetail, MediaFile } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { creditedNames, joinNames } from "./bookCredits";
import { chapterRows } from "./chapterRows";

function part(id: string, durationSeconds: number | undefined, available = true): MediaFile {
  return {
    id,
    fileName: `${id}.mp3`,
    sizeBytes: 1,
    modifiedAt: "2026-11-01T10:00:00Z",
    available,
    streams: [],
    ...(durationSeconds === undefined ? {} : { durationSeconds }),
  };
}

function book(fields: Partial<ItemDetail>): ItemDetail {
  return {
    id: "book-1",
    libraryId: "library-1",
    kind: "audiobook",
    title: "Book",
    files: [],
    ...fields,
  };
}

describe("chapterRows", () => {
  it("maps each chapter of a multi-file book to its file and local offset", () => {
    const rows = chapterRows(
      book({
        files: [part("one", 90), part("two", 3_600), part("three", undefined, false)],
        chapters: [
          { title: "Opening", startMs: 0, endMs: 90_000, fileId: "one" },
          { title: "Middle", startMs: 0, endMs: 1_800_000, fileId: "two" },
          { title: "Turn", startMs: 1_800_000, endMs: 3_600_000, fileId: "two" },
          { title: "End", startMs: 0, endMs: 61_500, fileId: "three" },
        ],
      }),
    );

    expect(rows).toEqual([
      {
        number: 1,
        title: "Opening",
        start: "0:00",
        duration: "1:30",
        fileId: "one",
        fileStartSeconds: undefined,
        available: true,
      },
      {
        number: 2,
        title: "Middle",
        start: "1:30",
        duration: "30:00",
        fileId: "two",
        fileStartSeconds: undefined,
        available: true,
      },
      {
        number: 3,
        title: "Turn",
        start: "31:30",
        duration: "30:00",
        fileId: "two",
        fileStartSeconds: 1_800,
        available: true,
      },
      {
        number: 4,
        title: "End",
        start: "1:01:30",
        duration: "1:01",
        fileId: "three",
        fileStartSeconds: undefined,
        available: false,
      },
    ]);
  });

  it("marks a chapter whose file the book does not list as unavailable", () => {
    const [row] = chapterRows(
      book({ chapters: [{ title: "Lost", startMs: 500, endMs: 1_000, fileId: "gone" }] }),
    );

    expect(row).toMatchObject({ fileId: "gone", fileStartSeconds: 0.5, available: false });
  });

  it("is empty for a book without chapters", () => {
    expect(chapterRows(book({ files: [part("one", 10)] }))).toEqual([]);
  });
});

describe("book credits", () => {
  const people = [
    { name: "Ada Inkwell", role: "author" as const },
    { name: "Nora Vale", role: "narrator" as const },
    { name: "Bram Quill", role: "author" as const },
  ];

  it("picks the names of a role in metadata order", () => {
    expect(creditedNames(people, "author")).toEqual(["Ada Inkwell", "Bram Quill"]);
    expect(creditedNames(people, "director")).toEqual([]);
    expect(creditedNames(undefined, "author")).toEqual([]);
  });

  it.each([
    { language: "en", names: ["A", "B", "C"], expected: "A, B, and C" },
    { language: "de", names: ["A", "B"], expected: "A und B" },
    { language: "en", names: ["A"], expected: "A" },
    { language: "en", names: [], expected: "" },
  ])("joins $names in $language", ({ language, names, expected }) => {
    expect(joinNames(names, language)).toBe(expected);
  });
});

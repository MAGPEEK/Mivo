import { describe, expect, it } from "vitest";

import { chooseAudio, chooseSubtitle, chooseVersion, resolveChoice } from "./playChoice";
import { maxRememberedChoices, readStoredChoice, writeStoredChoice } from "./playChoiceStorage";
import type { VersionChoices } from "./versions";

function version(fileId: string, defaults: Partial<VersionChoices> = {}): VersionChoices {
  return {
    fileId,
    label: fileId,
    audio: [
      { value: "1", label: "German" },
      { value: "2", label: "English" },
    ],
    subtitles: [{ value: "s:3", label: "English" }],
    defaultAudio: "1",
    defaultSubtitle: "",
    defaultSubtitleByAudio: { "1": "", "2": "s:3" },
    ...defaults,
  };
}

const versions = [version("a"), version("b", { defaultAudio: "2", defaultSubtitle: "s:3" })];

describe("resolveChoice", () => {
  it("shows the first version and its defaults for nothing stored", () => {
    expect(resolveChoice(versions, {})).toMatchObject({
      version: { fileId: "a" },
      audio: "1",
      subtitle: "",
      choice: {},
    });
  });

  it("has nothing to resolve without a version", () => {
    expect(resolveChoice([], {})).toBeUndefined();
  });

  it("keeps a stored pick and hands exactly the picks over", () => {
    expect(resolveChoice(versions, { fileId: "a", audio: "2", subtitle: "" })).toMatchObject({
      audio: "2",
      subtitle: "",
      choice: { audioStreamIndex: 2, subtitleTrackId: "" },
    });
  });

  it("drops stored tracks the version does not offer", () => {
    expect(resolveChoice(versions, { fileId: "a", audio: "9", subtitle: "s:9" })).toMatchObject({
      audio: "1",
      subtitle: "",
      choice: {},
    });
  });

  it("falls back to the first version for an unknown stored file", () => {
    expect(resolveChoice(versions, { fileId: "gone", audio: "2" })?.version.fileId).toBe("a");
  });

  it("does not carry tracks over from another version", () => {
    expect(resolveChoice(versions, { fileId: "b", audio: "1" })).toMatchObject({
      version: { fileId: "b" },
      audio: "1",
    });
    expect(chooseVersion("b")).toEqual({ fileId: "b" });
  });
});

describe("choosing", () => {
  it("starts afresh on another file and keeps the other pick on the same file", () => {
    expect(chooseAudio({ fileId: "a", subtitle: "s:3" }, "a", "2")).toEqual({
      fileId: "a",
      subtitle: "s:3",
      audio: "2",
    });
    expect(chooseSubtitle({ fileId: "a", audio: "2" }, "b", "s:3")).toEqual({
      fileId: "b",
      subtitle: "s:3",
    });
  });
});

describe("the subtitle follows the shown audio", () => {
  it("moves to the subtitle of the picked audio and hands it over explicitly", () => {
    expect(resolveChoice(versions, { fileId: "a", audio: "2" })).toMatchObject({
      audio: "2",
      subtitle: "s:3",
      choice: { audioStreamIndex: 2, subtitleTrackId: "s:3" },
    });
  });

  it("keeps an explicit subtitle pick whatever the audio", () => {
    expect(resolveChoice(versions, { fileId: "a", audio: "2", subtitle: "" })).toMatchObject({
      subtitle: "",
      choice: { audioStreamIndex: 2, subtitleTrackId: "" },
    });
  });
});

describe("storage", () => {
  it("reads only well-formed stored values", () => {
    const storage = window.localStorage;
    storage.setItem(
      "mivo.playChoices",
      JSON.stringify([
        { itemId: "x", choice: { fileId: 7, audio: "2", extra: true } },
        { itemId: 5, choice: {} },
        "junk",
      ]),
    );
    expect(readStoredChoice(storage, "x")).toEqual({ audio: "2" });
    storage.setItem("mivo.playChoices", "not json");
    expect(readStoredChoice(storage, "x")).toEqual({});
    storage.setItem("mivo.playChoices", "{}");
    expect(readStoredChoice(storage, "x")).toEqual({});
    expect(readStoredChoice(undefined, "x")).toEqual({});
  });

  it("keeps one key, newest first, and forgets the oldest beyond the cap", () => {
    const storage = window.localStorage;
    storage.clear();
    for (let item = 0; item < maxRememberedChoices + 5; item += 1) {
      writeStoredChoice(storage, `item-${String(item)}`, { audio: "1" });
    }
    writeStoredChoice(storage, "item-204", { audio: "2" });

    expect(storage.length).toBe(1);
    expect(readStoredChoice(storage, "item-204")).toEqual({ audio: "2" });
    expect(readStoredChoice(storage, "item-0")).toEqual({});
    expect(readStoredChoice(storage, "item-100")).toEqual({ audio: "1" });
    const entries = JSON.parse(storage.getItem("mivo.playChoices") ?? "[]") as unknown[];
    expect(entries).toHaveLength(maxRememberedChoices);
  });
});

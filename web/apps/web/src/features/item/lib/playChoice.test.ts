import { describe, expect, it } from "vitest";

import { chooseAudio, chooseSubtitle, chooseVersion, resolveChoice } from "./playChoice";
import { playChoiceState, readPlayChoice, readStoredChoice } from "./playChoiceStorage";
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

describe("storage and hand-over", () => {
  it("reads only well-formed stored values", () => {
    const storage = window.localStorage;
    storage.setItem("mivo.playChoice.x", JSON.stringify({ fileId: 7, audio: "2", extra: true }));
    expect(readStoredChoice(storage, "x")).toEqual({ audio: "2" });
    storage.setItem("mivo.playChoice.x", "not json");
    expect(readStoredChoice(storage, "x")).toEqual({});
    expect(readStoredChoice(undefined, "x")).toEqual({});
  });

  it("round-trips a choice through the history state and validates what it reads", () => {
    const state = playChoiceState({ audioStreamIndex: 2, subtitleTrackId: "s:3" });
    expect(readPlayChoice(state)).toEqual({ audioStreamIndex: 2, subtitleTrackId: "s:3" });
    expect(playChoiceState({})).toBeUndefined();
    expect(
      readPlayChoice({ mivoPlayChoice: { audioStreamIndex: -1, subtitleTrackId: 5 } }),
    ).toEqual({});
    expect(readPlayChoice(null)).toEqual({});
    expect(readPlayChoice({ mivoPlayChoice: "x" })).toEqual({});
  });
});

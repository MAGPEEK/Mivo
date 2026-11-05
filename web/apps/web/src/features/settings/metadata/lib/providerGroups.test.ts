import { describe, expect, it } from "vitest";

import type { MetadataSettings } from "../api/metadataSettings";
import { languageChoices, providerGroups, tmdbKeyStatus, toggleGroup } from "./providerGroups";

function settingsWith(providers: MetadataSettings["providers"]): MetadataSettings {
  return { onlineEnabled: true, language: "en", providers };
}

describe("providerGroups", () => {
  it.each([
    {
      name: "both groups, music on only when every member is on",
      providers: [
        { provider: "tmdb", enabled: false },
        { provider: "musicbrainz", enabled: true },
        { provider: "coverartarchive", enabled: false },
      ],
      expected: [
        { id: "tmdb", providers: ["tmdb"], enabled: false },
        { id: "music", providers: ["musicbrainz", "coverartarchive"], enabled: false },
      ],
    },
    {
      name: "a group without reported members is left out",
      providers: [{ provider: "musicbrainz", enabled: true }],
      expected: [{ id: "music", providers: ["musicbrainz"], enabled: true }],
    },
    { name: "no providers", providers: [], expected: [] },
  ] as const)("$name", ({ providers, expected }) => {
    expect(providerGroups(settingsWith([...providers]))).toEqual(expected);
  });

  it("toggles every member of a group", () => {
    expect(
      toggleGroup(
        { id: "music", providers: ["musicbrainz", "coverartarchive"], enabled: true },
        false,
      ),
    ).toEqual({
      providers: [
        { provider: "musicbrainz", enabled: false },
        { provider: "coverartarchive", enabled: false },
      ],
    });
  });
});

describe("tmdbKeyStatus", () => {
  it.each([
    { keyStatus: "bundled", expected: "bundled" },
    { keyStatus: "own", expected: "own" },
    { keyStatus: "none", expected: "none" },
    { keyStatus: undefined, expected: "none" },
  ] as const)("reads $keyStatus as $expected", ({ keyStatus, expected }) => {
    const tmdb =
      keyStatus === undefined
        ? { provider: "tmdb" as const, enabled: true }
        : { provider: "tmdb" as const, enabled: true, keyStatus };
    expect(tmdbKeyStatus(settingsWith([tmdb]))).toBe(expected);
  });

  it("reads a missing TMDB entry as none", () => {
    expect(tmdbKeyStatus(settingsWith([]))).toBe("none");
  });
});

describe("languageChoices", () => {
  it("adds the stored language when the list lacks it", () => {
    expect(languageChoices(["", "en"], "de-AT")).toEqual(["", "en", "de-AT"]);
    expect(languageChoices(["", "en"], "en")).toEqual(["", "en"]);
  });
});

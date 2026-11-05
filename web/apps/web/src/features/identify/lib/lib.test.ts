import { ApiError } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { candidatePosterSrc, candidateTitle } from "./candidate";
import { identifyErrorKind, isRetryable } from "./identifyError";
import {
  isMatchableKind,
  isProviderReference,
  parseSearchYear,
  providerName,
  toSearchRequest,
} from "./searchInput";

function apiError(status: number, code: string): ApiError {
  return new ApiError({ status, title: code, code });
}

describe("identifyErrorKind", () => {
  it.each([
    [apiError(503, "provider_unavailable"), "providerOffline"],
    [apiError(503, "unexpected_response"), "providerOffline"],
    [apiError(409, "online_disabled"), "onlineDisabled"],
    [apiError(501, "not_implemented"), "notSupported"],
    [apiError(501, "unexpected_response"), "notSupported"],
    [apiError(403, "forbidden"), "forbidden"],
    [apiError(400, "invalid"), "invalid"],
    [apiError(404, "not_found"), "notFound"],
    [apiError(0, "network_error"), "networkError"],
    [apiError(500, "internal"), "generic"],
    [apiError(500, "constructor"), "generic"],
    [new Error("boom"), "generic"],
  ] as const)("classifies %o as %s", (error, kind) => {
    expect(identifyErrorKind(error)).toBe(kind);
  });

  it("offers a retry only for transient failures", () => {
    expect(isRetryable("providerOffline")).toBe(true);
    expect(isRetryable("networkError")).toBe(true);
    expect(isRetryable("onlineDisabled")).toBe(false);
    expect(isRetryable("notSupported")).toBe(false);
  });
});

describe("isProviderReference", () => {
  it.each([
    ["https://www.themoviedb.org/movie/10378", true],
    ["  http://musicbrainz.org/release/abc  ", true],
    ["tmdb:10378", true],
    ["TMDB-10378", true],
    ["0383dadf-2a4e-4d10-a46a-e9e041da8eb3", true],
    ["1917", false],
    ["Big Buck Bunny", false],
    ["https://", false],
  ])("treats %j as reference: %s", (text, expected) => {
    expect(isProviderReference(text)).toBe(expected);
  });
});

describe("parseSearchYear", () => {
  it.each([
    ["2008", 2008],
    [" 1999 ", 1999],
    ["", undefined],
    ["08", undefined],
    ["1799", undefined],
    ["2201", undefined],
    ["20x8", undefined],
  ])("parses %j as %s", (text, expected) => {
    expect(parseSearchYear(text)).toBe(expected);
  });
});

describe("toSearchRequest", () => {
  it("searches by title and year", () => {
    expect(toSearchRequest({ query: " Big Buck Bunny ", year: "2008", itemTitle: "x" })).toEqual({
      query: "Big Buck Bunny",
      year: 2008,
    });
  });

  it("drops an invalid year", () => {
    expect(toSearchRequest({ query: "Bunny", year: "soon", itemTitle: "x" })).toEqual({
      query: "Bunny",
    });
  });

  it("looks up a pasted link with the item title as query and no year", () => {
    const link = "https://www.themoviedb.org/movie/10378";
    expect(toSearchRequest({ query: link, year: "2008", itemTitle: "Bunny" })).toEqual({
      query: "Bunny",
      providerId: link,
    });
  });

  it("falls back to the reference as query when the item has no title", () => {
    expect(toSearchRequest({ query: "tmdb:1", year: "", itemTitle: " " })).toEqual({
      query: "tmdb:1",
      providerId: "tmdb:1",
    });
  });

  it("has nothing to search for an empty query", () => {
    expect(toSearchRequest({ query: "  ", year: "2008", itemTitle: "Bunny" })).toBeUndefined();
  });

  it("cuts the query to the contract maximum", () => {
    const request = toSearchRequest({ query: "a".repeat(300), year: "", itemTitle: "" });
    expect(request?.query).toHaveLength(256);
  });
});

describe("isMatchableKind", () => {
  it.each([
    ["movie", true],
    ["show", true],
    ["album", true],
    ["artist", true],
    ["track", false],
    ["season", false],
    ["episode", false],
    ["extra", false],
  ] as const)("%s: %s", (kind, expected) => {
    expect(isMatchableKind(kind)).toBe(expected);
  });
});

describe("providerName", () => {
  it("names known providers and keeps unknown ones", () => {
    expect(providerName("tmdb")).toBe("TMDB");
    expect(providerName("musicbrainz")).toBe("MusicBrainz");
    expect(providerName("coverartarchive")).toBe("Cover Art Archive");
    expect(providerName("tvdb")).toBe("tvdb");
  });
});

describe("candidate", () => {
  const base = { provider: "tmdb", externalId: "1", title: "Bunny", confidence: 1 } as const;

  it("uses only server poster URLs", () => {
    expect(candidatePosterSrc({ ...base, posterUrl: "/api/v1/images/a?width=320" })).toBe(
      "/api/v1/images/a?width=320",
    );
    expect(candidatePosterSrc({ ...base, posterUrl: "https://image.tmdb.org/a.jpg" })).toBe(
      undefined,
    );
    expect(candidatePosterSrc(base)).toBeUndefined();
  });

  it("adds the year to the title when known", () => {
    expect(candidateTitle({ ...base, year: 2008 })).toBe("Bunny (2008)");
    expect(candidateTitle(base)).toBe("Bunny");
  });
});

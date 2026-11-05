import type { LibraryKind } from "@mivo/api-client";
import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { BrowseSearch } from "../lib/browseQuery";
import { useBrowseState, type BrowseSearchChange } from "./useBrowseState";

const fixedSeed = 1234;

function renderBrowse(search: BrowseSearch, libraryKind: LibraryKind = "movies") {
  const onSearchChange = vi.fn<BrowseSearchChange>();
  const nextSeed = () => fixedSeed;
  const rendered = renderHook(() =>
    useBrowseState({ search, onSearchChange, libraryKind, nextSeed }),
  );
  return { ...rendered, onSearchChange };
}

describe("useBrowseState", () => {
  it("derives the state and the listing query from the URL", () => {
    const { result } = renderBrowse({ type: "album", sort: "year", order: "desc" }, "music");

    expect(result.current).toMatchObject({
      type: "album",
      types: ["artist", "album"],
      sort: "year",
      order: "desc",
      filter: "all",
      query: { type: "album", sort: "year", order: "desc" },
    });
  });

  it("changes the sort and keeps the direction", () => {
    const { result, onSearchChange } = renderBrowse({ order: "desc" });

    act(() => {
      result.current.selectSort("dateAdded");
    });

    expect(onSearchChange).toHaveBeenCalledWith(
      expect.objectContaining({ sort: "dateAdded", order: "desc", seed: undefined }),
      undefined,
    );
  });

  it("gives the random sort a new seed on every choice", () => {
    const { result, onSearchChange } = renderBrowse({ sort: "random", seed: 1 });

    act(() => {
      result.current.selectSort("random");
    });

    expect(onSearchChange).toHaveBeenCalledWith(
      expect.objectContaining({ sort: "random", seed: fixedSeed }),
      undefined,
    );
  });

  it("adds a missing random seed to the URL without a new history entry", () => {
    const { result, onSearchChange } = renderBrowse({ sort: "random" });

    // No unseeded listing is requested while the seed is on its way into the URL.
    expect(result.current.query).toBeUndefined();
    expect(onSearchChange).toHaveBeenCalledExactlyOnceWith(
      expect.objectContaining({ sort: "random", seed: fixedSeed }),
      { replace: true },
    );
  });

  it("toggles the direction; ascending leaves the URL", () => {
    const { result, onSearchChange } = renderBrowse({ sort: "year", order: "desc" });

    act(() => {
      result.current.toggleOrder();
    });

    expect(onSearchChange).toHaveBeenCalledWith(
      expect.objectContaining({ sort: "year", order: undefined }),
      undefined,
    );
  });

  it("switches the type and keeps a sort both types offer", () => {
    const { result, onSearchChange } = renderBrowse({ sort: "dateAdded" }, "music");

    act(() => {
      result.current.selectType("artist");
    });
    expect(onSearchChange).not.toHaveBeenCalled();

    act(() => {
      result.current.selectType("album");
    });
    expect(onSearchChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ type: "album", sort: "dateAdded" }),
      undefined,
    );
  });

  it("resets an unoffered sort when the type changes", () => {
    const { result, onSearchChange } = renderBrowse(
      { type: "photo", sort: "resolution" },
      "photos",
    );

    act(() => {
      result.current.selectType("photo_album");
    });

    expect(onSearchChange).toHaveBeenCalledWith(
      expect.objectContaining({ type: undefined, sort: undefined }),
      undefined,
    );
  });

  it("ignores unknown types and sorts", () => {
    const { result, onSearchChange } = renderBrowse({});

    act(() => {
      result.current.selectType("album");
      result.current.selectSort("rating");
    });

    expect(onSearchChange).not.toHaveBeenCalled();
  });

  it("puts the unmatched filter into the URL", () => {
    const { result, onSearchChange } = renderBrowse({});

    act(() => {
      result.current.selectQuick("unmatched");
    });

    expect(onSearchChange).toHaveBeenCalledWith(
      expect.objectContaining({ matchState: "unmatched", quick: undefined }),
      undefined,
    );
  });

  it("replaces the unmatched filter with a server quick filter (single choice)", () => {
    const { result, onSearchChange } = renderBrowse({ matchState: "unmatched" });
    expect(result.current.filter).toBe("unmatched");

    act(() => {
      result.current.selectQuick("hdr");
    });

    expect(onSearchChange).toHaveBeenCalledWith(
      expect.objectContaining({ quick: "hdr", matchState: undefined }),
      undefined,
    );
  });

  it("ignores unknown and unchanged quick filters", () => {
    const { result, onSearchChange } = renderBrowse({ quick: "atmos" });

    act(() => {
      result.current.selectQuick("unwatched");
      result.current.selectQuick("atmos");
    });

    expect(onSearchChange).not.toHaveBeenCalled();
  });

  it("adds and removes field values and keeps the other filters", () => {
    const { result, onSearchChange } = renderBrowse({ quick: "uhd", genre: ["Drama"] });
    expect(result.current).toMatchObject({
      filter: "filtered",
      query: { quick: "uhd", genre: ["Drama"] },
      facetQuery: { quick: "uhd", genre: ["Drama"] },
    });

    act(() => {
      result.current.toggleFieldValue("genre", "Comedy");
    });
    expect(onSearchChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ quick: "uhd", genre: ["Drama", "Comedy"] }),
      undefined,
    );

    act(() => {
      result.current.toggleFieldValue("genre", "Drama");
    });
    expect(onSearchChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ quick: "uhd", genre: undefined }),
      undefined,
    );
  });

  it("clears every filter but keeps sort and type", () => {
    const { result, onSearchChange } = renderBrowse(
      { type: "album", sort: "year", quick: "hdr", decade: ["1990"] },
      "music",
    );

    act(() => {
      result.current.clearFilters();
    });

    expect(onSearchChange).toHaveBeenCalledWith(
      expect.objectContaining({
        type: "album",
        sort: "year",
        quick: undefined,
        matchState: undefined,
        decade: undefined,
      }),
      undefined,
    );
  });
});

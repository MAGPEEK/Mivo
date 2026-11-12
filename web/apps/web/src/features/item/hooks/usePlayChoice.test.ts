import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import type { VersionView } from "../lib/itemView";
import { usePlayChoice } from "./usePlayChoice";

function version(fileId: string): VersionView {
  return {
    fileId,
    label: fileId,
    audio: [
      { value: "1", label: "German" },
      { value: "2", label: "English" },
    ],
    subtitles: [],
    defaultAudio: "1",
    defaultSubtitle: "",
    defaultSubtitleByAudio: { "1": "", "2": "" },
    facts: [],
    chips: [],
    file: undefined,
    audioTracks: [],
    subtitleTracks: [],
  };
}

const versions = [version("file-1")];

describe("usePlayChoice", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("does not carry a pick over when the page shows another item", () => {
    const { result, rerender } = renderHook(({ itemId }) => usePlayChoice(itemId, versions), {
      initialProps: { itemId: "episode-1" },
    });
    act(() => {
      result.current.pickAudio("2");
    });
    expect(result.current.resolved?.audio).toBe("2");

    rerender({ itemId: "episode-2" });

    expect(result.current.resolved?.audio).toBe("1");
    expect(result.current.resolved?.choice).toEqual({});

    rerender({ itemId: "episode-1" });

    expect(result.current.resolved?.audio).toBe("2");
  });
});

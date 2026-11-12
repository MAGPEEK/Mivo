import { describe, expect, it } from "vitest";

import type { ItemView, VersionView } from "./itemView";
import { resolveChoice } from "./playChoice";
import { withChosenVersion } from "./shownVersion";

const second: VersionView = {
  fileId: "file-2",
  label: "4K",
  audio: [],
  subtitles: [],
  defaultAudio: undefined,
  defaultSubtitle: "",
  defaultSubtitleByAudio: {},
  facts: ["2 h", "4K"],
  chips: ["MP4"],
  file: { name: "b.mp4", size: "1 GB" },
  audioTracks: [],
  subtitleTracks: [],
};

const item = {
  facts: ["2024", "1 h", "1080p"],
  ownFacts: ["2024"],
  chips: ["MKV"],
  play: { kind: "playable", itemId: "item-1", fileId: "file-1" },
} as unknown as ItemView;

describe("withChosenVersion", () => {
  it("shows the facts, chips and file of the chosen version and plays it", () => {
    const shown = withChosenVersion(item, resolveChoice([second], { fileId: "file-2" }));

    expect(shown.facts).toEqual(["2024", "2 h", "4K"]);
    expect(shown.chips).toEqual(["MP4"]);
    expect(shown.play).toMatchObject({ fileId: "file-2" });
  });

  it("leaves the item alone without a version", () => {
    expect(withChosenVersion(item, undefined)).toBe(item);
  });
});

import { describe, expect, it } from "vitest";

import { choiceSelection, playChoiceState, readPlayChoice } from "./playChoiceHandoff";
import { playerTracks } from "./playerTracks";
import { englishSubtitleId, multiTrackItem, playerFileId } from "../test/playerFixtures";

const tracks = playerTracks(multiTrackItem(), playerFileId);

describe("play choice hand-over", () => {
  it("round-trips a choice for its item", () => {
    const state = playChoiceState("item-1", { audioStreamIndex: 2, subtitleTrackId: "s:5" });

    expect(readPlayChoice(state, "item-1")).toEqual({
      audioStreamIndex: 2,
      subtitleTrackId: "s:5",
    });
  });

  it("carries Off as the empty subtitle id and nothing for an empty choice", () => {
    expect(readPlayChoice(playChoiceState("item-1", { subtitleTrackId: "" }), "item-1")).toEqual({
      subtitleTrackId: "",
    });
    expect(playChoiceState("item-1", {})).toBeUndefined();
  });

  it("ignores a choice made for another item", () => {
    const state = playChoiceState("item-1", { audioStreamIndex: 2 });

    expect(readPlayChoice(state, "item-2")).toEqual({});
  });

  const invalid: readonly (readonly [string, unknown])[] = [
    ["no state", undefined],
    ["a string state", "x"],
    ["a non-object handoff", { mivoPlayChoice: "x" }],
    ["a null handoff", { mivoPlayChoice: null }],
    ["a fractional index", { mivoPlayChoice: { itemId: "item-1", audioStreamIndex: 1.5 } }],
    ["a negative index", { mivoPlayChoice: { itemId: "item-1", audioStreamIndex: -1 } }],
    ["a huge index", { mivoPlayChoice: { itemId: "item-1", audioStreamIndex: 1000 } }],
    ["a string index", { mivoPlayChoice: { itemId: "item-1", audioStreamIndex: "2" } }],
    ["a bad track id", { mivoPlayChoice: { itemId: "item-1", subtitleTrackId: "../x" } }],
    ["a numeric track id", { mivoPlayChoice: { itemId: "item-1", subtitleTrackId: 5 } }],
    [
      "an overlong track id",
      { mivoPlayChoice: { itemId: "item-1", subtitleTrackId: `x:${"a".repeat(200)}` } },
    ],
    [
      "one valid and one invalid field",
      { mivoPlayChoice: { itemId: "item-1", audioStreamIndex: 2, subtitleTrackId: "nope" } },
    ],
  ];
  it.each(invalid)("ignores the whole state for %s", (_name, state) => {
    expect(readPlayChoice(state, "item-1")).toEqual({});
  });
});

describe("choiceSelection", () => {
  it("keeps the tracks the file has, Off included", () => {
    expect(
      choiceSelection({ audioStreamIndex: 2, subtitleTrackId: englishSubtitleId }, tracks),
    ).toEqual({ audioStreamIndex: 2, subtitleTrackId: englishSubtitleId });
    expect(choiceSelection({ subtitleTrackId: "" }, tracks)).toEqual({ subtitleTrackId: "" });
  });

  it("drops unknown tracks and leaves them to the policy", () => {
    expect(choiceSelection({ audioStreamIndex: 9, subtitleTrackId: "s:99" }, tracks)).toEqual({});
    expect(choiceSelection({ audioStreamIndex: 2, subtitleTrackId: "s:99" }, tracks)).toEqual({
      audioStreamIndex: 2,
    });
  });

  it("selects nothing without a choice or tracks", () => {
    expect(choiceSelection(undefined, tracks)).toEqual({});
    expect(choiceSelection({ audioStreamIndex: 2 }, undefined)).toEqual({});
  });
});

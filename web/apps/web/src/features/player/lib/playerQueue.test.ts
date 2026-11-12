import { setQueue, setRepeat, setShuffle } from "@mivo/player-core";
import { describe, expect, it } from "vitest";

import {
  canSkipToNext,
  currentItemId,
  hasQueue,
  isShuffled,
  nextRepeatMode,
  queueFromHandoff,
  singleItemQueue,
} from "./playerQueue";

describe("queueFromHandoff", () => {
  const ids = ["a", "b", "c", "d"];

  it("starts at the opened item and keeps the order", () => {
    const queue = queueFromHandoff({ ids, shuffle: false }, "c", 1);

    expect(queue.items.map((entry) => entry.id)).toEqual(ids);
    expect(currentItemId(queue)).toBe("c");
    expect(isShuffled(queue)).toBe(false);
  });

  it("plays the opened item first and every item once when shuffled", () => {
    const queue = queueFromHandoff({ ids, shuffle: true }, "b", 7);

    expect(currentItemId(queue)).toBe("b");
    expect(queue.currentIndex).toBe(0);
    expect(isShuffled(queue)).toBe(true);
    expect(queue.items.map((entry) => entry.id).sort()).toEqual(ids);
  });
});

const threeItems = setQueue(singleItemQueue("a"), [
  { id: "a", meta: {} },
  { id: "b", meta: {} },
  { id: "c", meta: {} },
]);

describe("nextRepeatMode", () => {
  it.each([
    { from: "off", to: "all" },
    { from: "all", to: "one" },
    { from: "one", to: "off" },
  ] as const)("goes from $from to $to", ({ from, to }) => {
    expect(nextRepeatMode(from)).toBe(to);
  });
});

describe("player queue state", () => {
  it("holds only the opened item at first", () => {
    const queue = singleItemQueue("a");

    expect(currentItemId(queue)).toBe("a");
    expect(hasQueue(queue)).toBe(false);
    expect(canSkipToNext(queue)).toBe(false);
  });

  it("skips to next until the end unless repeat all wraps", () => {
    const atEnd = { ...threeItems, currentIndex: 2 };

    expect(canSkipToNext(threeItems)).toBe(true);
    expect(canSkipToNext(atEnd)).toBe(false);
    expect(canSkipToNext(setRepeat(atEnd, "all"))).toBe(true);
  });

  it("reports shuffle", () => {
    expect(isShuffled(threeItems)).toBe(false);
    expect(isShuffled(setShuffle(threeItems, true, 1))).toBe(true);
  });
});

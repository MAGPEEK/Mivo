import { describe, expect, it } from "vitest";

import {
  addToQueue,
  clearUpcoming,
  currentEntry,
  emptyQueue,
  jumpTo,
  moveItem,
  next,
  playNext,
  previous,
  removeAt,
  setQueue,
  setRepeat,
  setShuffle,
} from "./queue";
import { parseQueue, serializeQueue } from "./serialize";
import type { QueueItem, QueueState } from "./queueTypes";
import { seededRandom, shuffled } from "./shuffle";

const item = (id: string): QueueItem<{ title: string }> => ({ id, meta: { title: id } });
const items = (...ids: string[]) => ids.map(item);
const ids = (state: QueueState<unknown>) => state.items.map((entry) => entry.id);
const queueOf = (list: string[], start = 0) =>
  setQueue(emptyQueue<{ title: string }>(), items(...list), start);

describe("setQueue", () => {
  it.each([
    { name: "start index", list: ["a", "b", "c"], start: 2, expected: 2 },
    { name: "invalid start falls back to 0", list: ["a", "b"], start: 9, expected: 0 },
    { name: "negative start", list: ["a"], start: -1, expected: 0 },
    { name: "empty list", list: [], start: 0, expected: -1 },
  ])("$name", ({ list, start, expected }) => {
    expect(queueOf(list, start).currentIndex).toBe(expected);
  });

  it("keeps repeat and resets shuffle", () => {
    const shuffledState = setShuffle(setRepeat(queueOf(["a", "b"]), "all"), true, 1);
    const replaced = setQueue(shuffledState, items("x", "y"));
    expect(replaced.repeat).toBe("all");
    expect(replaced.originalKeys).toBeNull();
  });
});

describe("add and playNext", () => {
  it("appends and inserts after the current item", () => {
    const state = queueOf(["a", "b", "c"], 1);
    expect(ids(addToQueue(state, items("x")))).toEqual(["a", "b", "c", "x"]);
    expect(ids(playNext(state, items("x", "y")))).toEqual(["a", "b", "x", "y", "c"]);
  });

  it("makes the first added item current in an empty queue", () => {
    const state = addToQueue(emptyQueue<{ title: string }>(), items("a"));
    expect(state.currentIndex).toBe(0);
  });

  it("ignores an empty addition", () => {
    const state = queueOf(["a"]);
    expect(addToQueue(state, [])).toBe(state);
  });

  it("allows duplicates with distinct keys", () => {
    const state = addToQueue(queueOf(["a"]), items("a", "a"));
    expect(ids(state)).toEqual(["a", "a", "a"]);
    expect(new Set(state.items.map((entry) => entry.key)).size).toBe(3);
  });
});

describe("removeAt", () => {
  it.each([
    { name: "before current", index: 0, current: 2, expectedIds: ["b", "c"], expectedIndex: 1 },
    {
      name: "current in the middle",
      index: 1,
      current: 1,
      expectedIds: ["a", "c"],
      expectedIndex: 1,
    },
    { name: "current at the end", index: 2, current: 2, expectedIds: ["a", "b"], expectedIndex: 1 },
    { name: "after current", index: 2, current: 0, expectedIds: ["a", "b"], expectedIndex: 0 },
    { name: "out of range", index: 7, current: 1, expectedIds: ["a", "b", "c"], expectedIndex: 1 },
  ])("$name", ({ index, current, expectedIds, expectedIndex }) => {
    const state = removeAt(queueOf(["a", "b", "c"], current), index);
    expect(ids(state)).toEqual(expectedIds);
    expect(state.currentIndex).toBe(expectedIndex);
  });

  it("removing the only item empties the queue", () => {
    const state = removeAt(queueOf(["a"]), 0);
    expect(state.currentIndex).toBe(-1);
    expect(currentEntry(state)).toBeUndefined();
  });

  it("removes only the chosen duplicate", () => {
    const state = removeAt(queueOf(["a", "a", "b"], 2), 0);
    expect(ids(state)).toEqual(["a", "b"]);
    expect(currentEntry(state)?.id).toBe("b");
  });
});

describe("moveItem", () => {
  it.each([
    { from: 0, to: 2, expectedIds: ["b", "c", "a"] },
    { from: 2, to: 0, expectedIds: ["c", "a", "b"] },
  ])("moves $from to $to and keeps the current item", ({ from, to, expectedIds }) => {
    const state = moveItem(queueOf(["a", "b", "c"], 1), from, to);
    expect(ids(state)).toEqual(expectedIds);
    expect(currentEntry(state)?.id).toBe("b");
  });

  it.each([
    [0, 0],
    [-1, 1],
    [0, 5],
  ])("ignores %i -> %i", (from, to) => {
    const state = queueOf(["a", "b"]);
    expect(moveItem(state, from, to)).toBe(state);
  });
});

describe("clearUpcoming and jumpTo", () => {
  it("drops items after the current one", () => {
    expect(ids(clearUpcoming(queueOf(["a", "b", "c"], 1)))).toEqual(["a", "b"]);
  });

  it("clears an empty queue safely", () => {
    expect(clearUpcoming(emptyQueue()).items).toEqual([]);
  });

  it("jumps within range only", () => {
    const state = queueOf(["a", "b", "c"]);
    expect(jumpTo(state, 2).currentIndex).toBe(2);
    expect(jumpTo(state, 3)).toBe(state);
    expect(jumpTo(state, 1.5)).toBe(state);
  });
});

describe("next", () => {
  it.each([
    { name: "middle", repeat: "off", current: 0, user: false, kind: "moved", index: 1 },
    { name: "end, repeat off", repeat: "off", current: 2, user: false, kind: "ended", index: 2 },
    {
      name: "end, repeat all wraps",
      repeat: "all",
      current: 2,
      user: false,
      kind: "moved",
      index: 0,
    },
    { name: "repeat one stays", repeat: "one", current: 1, user: false, kind: "moved", index: 1 },
    {
      name: "repeat one, user skips",
      repeat: "one",
      current: 1,
      user: true,
      kind: "moved",
      index: 2,
    },
    {
      name: "repeat one, user at end",
      repeat: "one",
      current: 2,
      user: true,
      kind: "moved",
      index: 0,
    },
  ] as const)("$name", ({ repeat, current, user, kind, index }) => {
    const result = next(setRepeat(queueOf(["a", "b", "c"], current), repeat), user);
    expect(result.kind).toBe(kind);
    expect(result.state.currentIndex).toBe(index);
  });

  it("ends on an empty queue", () => {
    expect(next(emptyQueue()).kind).toBe("ended");
  });
});

describe("previous", () => {
  it.each([
    {
      name: "after 3 s restarts",
      repeat: "off",
      current: 1,
      position: 3.5,
      index: 1,
      restart: true,
    },
    {
      name: "within 3 s goes back",
      repeat: "off",
      current: 1,
      position: 3,
      index: 0,
      restart: false,
    },
    {
      name: "first item restarts",
      repeat: "off",
      current: 0,
      position: 1,
      index: 0,
      restart: true,
    },
    {
      name: "first item, repeat all wraps",
      repeat: "all",
      current: 0,
      position: 1,
      index: 2,
      restart: false,
    },
  ] as const)("$name", ({ repeat, current, position, index, restart }) => {
    const result = previous(setRepeat(queueOf(["a", "b", "c"], current), repeat), position);
    expect(result.state.currentIndex).toBe(index);
    expect(result.restartCurrent).toBe(restart);
  });

  it("does not restart an empty queue", () => {
    expect(previous(emptyQueue(), 10).restartCurrent).toBe(false);
  });
});

describe("shuffle", () => {
  const list = ["a", "b", "c", "d", "e", "f", "g", "h"];

  it("is deterministic per seed and keeps the current item first", () => {
    const state = queueOf(list, 3);
    const first = setShuffle(state, true, 42);
    expect(ids(first)).toEqual(ids(setShuffle(state, true, 42)));
    expect(first.currentIndex).toBe(0);
    expect(first.items[0]?.id).toBe("d");
    expect([...ids(first)].sort()).toEqual(list);
    expect(ids(setShuffle(state, true, 43))).not.toEqual(ids(first));
  });

  it("restores the original order and the current item when turned off", () => {
    const shuffledState = jumpTo(setShuffle(queueOf(list, 3), true, 7), 4);
    const current = currentEntry(shuffledState)?.id;
    const restored = setShuffle(shuffledState, false, 7);
    expect(ids(restored)).toEqual(list);
    expect(currentEntry(restored)?.id).toBe(current);
    expect(restored.originalKeys).toBeNull();
  });

  it("restores duplicates in their original positions", () => {
    const dup = ["a", "b", "a", "b", "a"];
    expect(ids(setShuffle(setShuffle(queueOf(dup, 2), true, 5), false, 5))).toEqual(dup);
  });

  it("appends items added while shuffled and drops removed ones when restoring", () => {
    const shuffledState = addToQueue(setShuffle(queueOf(["a", "b", "c"]), true, 9), items("x"));
    const removed = removeAt(
      shuffledState,
      shuffledState.items.findIndex((entry) => entry.id === "b"),
    );
    expect(ids(setShuffle(removed, false, 9))).toEqual(["a", "c", "x"]);
  });

  it("is a no-op when the state already matches", () => {
    const state = queueOf(list);
    expect(setShuffle(state, false, 1)).toBe(state);
    const on = setShuffle(state, true, 1);
    expect(setShuffle(on, true, 2)).toBe(on);
  });

  it("handles an empty queue", () => {
    const state = setShuffle(emptyQueue(), true, 1);
    expect(state.currentIndex).toBe(-1);
    expect(setShuffle(state, false, 1).currentIndex).toBe(-1);
  });
});

describe("seeded random", () => {
  it("repeats for a seed and stays in [0, 1)", () => {
    const a = seededRandom(1);
    const b = seededRandom(1);
    for (let count = 0; count < 20; count += 1) {
      const value = a();
      expect(value).toBe(b());
      expect(value).toBeGreaterThanOrEqual(0);
      expect(value).toBeLessThan(1);
    }
    expect(shuffled([1, 2, 3, 4, 5], 3)).toEqual(shuffled([1, 2, 3, 4, 5], 3));
  });
});

describe("serialisation", () => {
  it("round-trips including shuffle state", () => {
    const state = setShuffle(setRepeat(queueOf(["a", "b", "a"], 1), "one"), true, 4);
    expect(parseQueue(serializeQueue(state))).toEqual(state);
  });

  it.each([
    "not json",
    "null",
    "[]",
    "{}",
    '{"items":[{"id":1,"key":0}],"currentIndex":0,"repeat":"off","originalKeys":null,"nextKey":1}',
    '{"items":[{"id":"a","key":0}],"currentIndex":3,"repeat":"off","originalKeys":null,"nextKey":1}',
    '{"items":[],"currentIndex":0,"repeat":"off","originalKeys":null,"nextKey":0}',
    '{"items":[{"id":"a","key":0}],"currentIndex":0,"repeat":"loop","originalKeys":null,"nextKey":1}',
    '{"items":[{"id":"a","key":0},{"id":"b","key":0}],"currentIndex":0,"repeat":"off","originalKeys":null,"nextKey":1}',
    '{"items":[{"id":"a","key":5}],"currentIndex":0,"repeat":"off","originalKeys":null,"nextKey":1}',
    '{"items":[{"id":"a","key":0}],"currentIndex":0,"repeat":"off","originalKeys":"x","nextKey":1}',
  ])("invalid input %s yields an empty queue", (json) => {
    expect(parseQueue(json)).toEqual(emptyQueue());
  });
});

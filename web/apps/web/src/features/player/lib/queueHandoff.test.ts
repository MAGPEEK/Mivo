import { describe, expect, it } from "vitest";

import { maxHandoffIds, queueHandoffState, readQueueHandoff } from "./queueHandoff";

describe("queueHandoff", () => {
  it("round-trips through the history state", () => {
    const state = queueHandoffState({ ids: ["a", "b", "c"], shuffle: true });

    expect(readQueueHandoff(state, "b")).toEqual({ ids: ["a", "b", "c"], shuffle: true });
  });

  it.each([
    ["no state", undefined],
    ["foreign state", { other: 1 }],
    ["ids not a list", { mivoQueue: { ids: "a", shuffle: false } }],
    ["ids with a non-string", { mivoQueue: { ids: ["a", 2], shuffle: false } }],
    ["shuffle missing", { mivoQueue: { ids: ["a"] } }],
    ["an empty id", { mivoQueue: { ids: ["a", ""], shuffle: false } }],
    ["an id with foreign characters", { mivoQueue: { ids: ["a", "b/../c"], shuffle: false } }],
    ["an id that is too long", { mivoQueue: { ids: ["a", "x".repeat(65)], shuffle: false } }],
    ["duplicate ids", { mivoQueue: { ids: ["a", "b", "a"], shuffle: false } }],
    [
      "too many ids",
      {
        mivoQueue: {
          ids: Array.from({ length: maxHandoffIds + 1 }, (_, index) => `id-${String(index)}`),
          shuffle: false,
        },
      },
    ],
  ])("ignores %s", (_name, state) => {
    expect(readQueueHandoff(state, "a")).toBeUndefined();
  });

  it("ignores a queue that does not hold the opened item (stale state after navigation)", () => {
    const state = queueHandoffState({ ids: ["a", "b"], shuffle: false });

    expect(readQueueHandoff(state, "z")).toBeUndefined();
  });
});

import { describe, expect, it } from "vitest";

import { queueHandoffState, readQueueHandoff } from "./queueHandoff";

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
  ])("ignores %s", (_name, state) => {
    expect(readQueueHandoff(state, "a")).toBeUndefined();
  });

  it("ignores a queue that does not hold the opened item (stale state after navigation)", () => {
    const state = queueHandoffState({ ids: ["a", "b"], shuffle: false });

    expect(readQueueHandoff(state, "z")).toBeUndefined();
  });
});

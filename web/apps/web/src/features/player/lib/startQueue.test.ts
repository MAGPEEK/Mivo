import { describe, expect, it } from "vitest";

import { StartQueue } from "./startQueue";

describe("StartQueue", () => {
  it("lets one wish through and keeps only the newest one behind it", () => {
    const queue = new StartQueue<string>();

    expect(queue.tryBegin("a")).toBe(true);
    expect(queue.tryBegin("b")).toBe(false);
    expect(queue.tryBegin("c")).toBe(false);

    expect(queue.finish()).toBe("c");
    expect(queue.tryBegin("c")).toBe(true);
    expect(queue.finish()).toBeUndefined();
  });

  it("drops the waiting wish on clear but stays busy until the request settles", () => {
    const queue = new StartQueue<string>();
    queue.tryBegin("a");
    queue.tryBegin("b");

    queue.clear();

    expect(queue.tryBegin("c")).toBe(false);
    expect(queue.finish()).toBe("c");
  });
});

import type { TimeRange } from "@mivo/player-core";
import { describe, expect, it, vi } from "vitest";

import { fakeSession } from "../test/playerFixtures";
import {
  createMediaStateStore,
  initialMediaState,
  readMediaState,
  type MediaStateSource,
} from "./mediaState";

class FakeSource extends EventTarget implements MediaStateSource {
  paused = true;
  muted = false;
  volume = 1;
  ranges: TimeRange[] = [];

  get seekable() {
    const { ranges } = this;
    return {
      length: ranges.length,
      start: (index: number) => ranges[index]?.start ?? Number.NaN,
      end: (index: number) => ranges[index]?.end ?? Number.NaN,
    };
  }
}

const hlsSession = fakeSession({ id: "s", delivery: "hls", startPositionSeconds: 60 });

describe("readMediaState", () => {
  it("keeps the previous state while nothing changed", () => {
    const source = new FakeSource();

    expect(readMediaState(source, undefined, initialMediaState)).toBe(initialMediaState);
  });

  it("reads a changed element and keeps equal ranges as they were", () => {
    const source = new FakeSource();
    source.ranges = [{ start: 0, end: 12 }];
    const first = readMediaState(source, hlsSession, initialMediaState);
    source.paused = false;

    const second = readMediaState(source, hlsSession, first);

    expect(first.producedRanges).toEqual([{ start: 60, end: 72 }]);
    expect(second).toMatchObject({ paused: false });
    expect(second.producedRanges).toBe(first.producedRanges);
  });

  it.each([
    { change: (source: FakeSource) => (source.muted = true) },
    { change: (source: FakeSource) => (source.volume = 0.3) },
  ])("notices a change of the sound ($change)", ({ change }) => {
    const source = new FakeSource();
    change(source);

    expect(readMediaState(source, undefined, initialMediaState)).not.toBe(initialMediaState);
  });
});

describe("createMediaStateStore", () => {
  it("notifies on the element's events until unsubscribed", () => {
    const source = new FakeSource();
    const store = createMediaStateStore(source, undefined);
    const listener = vi.fn();

    const unsubscribe = store.subscribe(listener);
    source.dispatchEvent(new Event("volumechange"));
    unsubscribe();
    source.dispatchEvent(new Event("volumechange"));

    expect(listener).toHaveBeenCalledOnce();
  });

  it("shows the initial state without an element", () => {
    const store = createMediaStateStore(null, undefined);

    store.subscribe(vi.fn())();

    expect(store.getSnapshot()).toBe(initialMediaState);
  });
});

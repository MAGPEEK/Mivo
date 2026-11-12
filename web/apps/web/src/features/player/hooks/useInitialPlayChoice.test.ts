import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { playChoiceState } from "../lib/playChoiceHandoff";
import { useInitialPlayChoice } from "./useInitialPlayChoice";

let historyState: unknown = {};

vi.mock("@tanstack/react-router", () => {
  const router = {
    state: {
      location: {
        get state() {
          return historyState;
        },
      },
    },
  };
  return { useRouter: () => router };
});

afterEach(() => {
  historyState = {};
});

describe("useInitialPlayChoice", () => {
  it("reads the choice handed over for the item", () => {
    historyState = playChoiceState("a", { audioStreamIndex: 2 });

    const { result } = renderHook(() => useInitialPlayChoice("a"));

    expect(result.current).toEqual({ audioStreamIndex: 2 });
  });

  it("keeps the same object while the item stays, so the playback is not reopened", () => {
    historyState = playChoiceState("a", { audioStreamIndex: 2 });
    const { result, rerender } = renderHook(() => useInitialPlayChoice("a"));
    const first = result.current;

    rerender();

    expect(result.current).toBe(first);
  });

  it("reads the state again on a fresh mount, as after a reload", () => {
    historyState = playChoiceState("a", { audioStreamIndex: 2, subtitleTrackId: "" });
    const first = renderHook(() => useInitialPlayChoice("a"));
    first.unmount();

    const second = renderHook(() => useInitialPlayChoice("a"));

    expect(second.result.current).toEqual({ audioStreamIndex: 2, subtitleTrackId: "" });
  });

  it("does not apply the choice to the next item of a queue", () => {
    historyState = playChoiceState("a", { audioStreamIndex: 2 });
    const { result, rerender } = renderHook(({ itemId }) => useInitialPlayChoice(itemId), {
      initialProps: { itemId: "a" },
    });

    rerender({ itemId: "b" });

    expect(result.current).toEqual({});
  });

  it("ignores an invalid state", () => {
    historyState = { mivoPlayChoice: { itemId: "a", audioStreamIndex: "2" } };

    const { result } = renderHook(() => useInitialPlayChoice("a"));

    expect(result.current).toEqual({});
  });
});

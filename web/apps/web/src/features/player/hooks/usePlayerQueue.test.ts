import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { PlayerQueueItem } from "../lib/playerQueue";
import { queueHandoffState } from "../lib/queueHandoff";
import { usePlayerQueue, type PlayerQueueOptions } from "./usePlayerQueue";

const navigate = vi.fn<(options: unknown) => Promise<void>>();
let historyState: unknown = {};

vi.mock("@tanstack/react-router", () => ({
  useRouter: () => ({ navigate, state: { location: { state: historyState } } }),
}));

const items: readonly PlayerQueueItem[] = [
  { id: "a", meta: {} },
  { id: "b", meta: {} },
  { id: "c", meta: {} },
];

function renderQueue(initial: Partial<PlayerQueueOptions> = {}) {
  const restart = vi.fn<() => void>();
  const rendered = renderHook((props: PlayerQueueOptions) => usePlayerQueue(props), {
    initialProps: { itemId: "a", position: 0, restart, ...initial },
  });
  return { ...rendered, restart };
}

function navigatedTo(itemId: string) {
  return {
    to: "/items/$itemId/play",
    params: { itemId },
    search: {},
    replace: true,
  };
}

beforeEach(() => {
  navigate.mockResolvedValue(undefined);
});

afterEach(() => {
  navigate.mockReset();
  historyState = {};
});

describe("usePlayerQueue", () => {
  it("starts with the queue a page handed over in the history state", () => {
    historyState = queueHandoffState({ ids: ["a", "b", "c"], shuffle: false });

    const { result } = renderQueue({ itemId: "b" });

    expect(result.current.view).toMatchObject({ canGoPrevious: true, canGoNext: true });
  });

  it("offers neither previous nor next for the opened item alone", () => {
    const { result } = renderQueue();

    expect(result.current.view).toMatchObject({ canGoPrevious: false, canGoNext: false });
  });

  it("opens the next item, replacing the history entry", () => {
    const { result, rerender, restart } = renderQueue();
    act(() => {
      result.current.replace(items);
    });

    act(() => {
      result.current.next();
    });
    expect(navigate).toHaveBeenLastCalledWith(navigatedTo("b"));
    rerender({ itemId: "b", position: 0, restart });

    expect(result.current.view).toMatchObject({ canGoPrevious: true, canGoNext: true });
  });

  it("opens the previous item early in the item and restarts it later", () => {
    const { result, rerender, restart } = renderQueue();
    act(() => {
      result.current.replace(items, 1);
    });
    expect(navigate).toHaveBeenLastCalledWith(navigatedTo("b"));
    rerender({ itemId: "b", position: 30, restart });

    act(() => {
      result.current.previous();
    });
    expect(restart).toHaveBeenCalledOnce();

    rerender({ itemId: "b", position: 1, restart });
    act(() => {
      result.current.previous();
    });
    expect(navigate).toHaveBeenLastCalledWith(navigatedTo("a"));
  });

  it("does nothing on next at the end without repeat", () => {
    const { result, rerender, restart } = renderQueue();
    act(() => {
      result.current.replace(items, 2);
    });
    rerender({ itemId: "c", position: 0, restart });
    navigate.mockClear();

    act(() => {
      result.current.next();
    });

    expect(navigate).not.toHaveBeenCalled();
    expect(result.current.view.canGoNext).toBe(false);
  });

  it("starts over with the new item when the route opens another item", () => {
    const { result, rerender, restart } = renderQueue();
    act(() => {
      result.current.replace(items);
    });

    rerender({ itemId: "z", position: 0, restart });

    expect(result.current.view.canGoNext).toBe(false);
  });

  it("cycles repeat and toggles shuffle, remembering what changed last", () => {
    const { result } = renderQueue();

    act(() => {
      result.current.cycleRepeat();
    });
    expect(result.current.view).toMatchObject({ repeat: "all", announcement: "repeat" });
    act(() => {
      result.current.toggleShuffle();
    });
    expect(result.current.view).toMatchObject({ shuffle: true, announcement: "shuffle" });
  });
});

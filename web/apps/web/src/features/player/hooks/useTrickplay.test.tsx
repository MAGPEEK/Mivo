import type { TrickplayLayout } from "@mivo/player-core";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { ApiClientProvider } from "../../../shared/api";
import { createFakeApiClient } from "../../../test/fakeApi";
import type { TrickplayApi } from "../api/trickplayApi";
import { useTrickplayTile, type TrickplayServices } from "./useTrickplay";

const layout: TrickplayLayout = {
  tileWidth: 160,
  tileHeight: 90,
  columns: 10,
  rows: 10,
  intervalSeconds: 10,
  sheetUrls: ["/api/v1/media-files/f1/trickplay/0.jpg"],
};

function wrapper({ children }: { readonly children: ReactNode }) {
  return <ApiClientProvider client={createFakeApiClient()}>{children}</ApiClientProvider>;
}

function setup(manifests: (TrickplayLayout | undefined)[]) {
  const signals: AbortSignal[] = [];
  const getManifest = vi.fn<TrickplayApi["getManifest"]>((_fileId, signal) => {
    signals.push(signal);
    return Promise.resolve(manifests.length > 1 ? manifests.shift() : manifests[0]);
  });
  const requestGeneration = vi.fn<TrickplayApi["requestGeneration"]>(() => Promise.resolve());
  const releaseDelay: (() => void)[] = [];
  const services: TrickplayServices = {
    api: { getManifest, requestGeneration },
    loadSheet: () => ({ result: Promise.resolve(true), cancel: vi.fn() }),
    waits: {
      delay: () =>
        new Promise<void>((resolve) => {
          releaseDelay.push(resolve);
        }),
      visible: () => Promise.resolve(),
    },
  };
  return { services, getManifest, requestGeneration, signals, releaseDelay };
}

describe("useTrickplayTile", () => {
  it("asks for the manifest once per file and generates once on a 404", async () => {
    const { services, getManifest, requestGeneration } = setup([undefined]);
    const { rerender } = renderHook(({ seconds }) => useTrickplayTile("f1", seconds, services), {
      wrapper,
      initialProps: { seconds: undefined as number | undefined },
    });

    await waitFor(() => {
      expect(requestGeneration).toHaveBeenCalledTimes(1);
    });
    rerender({ seconds: 30 });
    rerender({ seconds: 40 });

    expect(getManifest).toHaveBeenCalledTimes(1);
    expect(requestGeneration).toHaveBeenCalledTimes(1);
  });

  it("shows previews after the generation without a reload", async () => {
    const { services, getManifest, releaseDelay } = setup([undefined, layout]);
    const { result } = renderHook(() => useTrickplayTile("f1", 95, services), { wrapper });
    await waitFor(() => {
      expect(releaseDelay).toHaveLength(1);
    });
    expect(result.current).toBeUndefined();

    releaseDelay[0]?.();

    await waitFor(() => {
      expect(result.current?.tile).toMatchObject({ sheet: 0, column: 9, row: 0 });
    });
    expect(result.current?.sheetUrl).toContain("/api/v1/media-files/f1/trickplay/0.jpg");
    expect(getManifest).toHaveBeenCalledTimes(2);
  });

  it("cancels the polling when it unmounts", async () => {
    const { services, signals, releaseDelay } = setup([undefined]);
    const { unmount } = renderHook(() => useTrickplayTile("f1", undefined, services), { wrapper });
    await waitFor(() => {
      expect(releaseDelay).toHaveLength(1);
    });

    unmount();

    expect(signals[0]?.aborted).toBe(true);
  });

  it("asks for nothing without a file", () => {
    const { services, getManifest } = setup([layout]);
    renderHook(() => useTrickplayTile(undefined, 10, services), { wrapper });

    expect(getManifest).not.toHaveBeenCalled();
  });
});

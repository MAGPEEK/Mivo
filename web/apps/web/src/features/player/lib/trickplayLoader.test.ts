import { trickplayPollMaxAttempts, type TrickplayLayout } from "@mivo/player-core";
import { describe, expect, it, vi } from "vitest";

import type { TrickplayApi } from "../api/trickplayApi";
import { browserWaits, loadTrickplay, type TrickplayLoaderWaits } from "./trickplayLoader";

const layout: TrickplayLayout = {
  tileWidth: 160,
  tileHeight: 90,
  columns: 10,
  rows: 10,
  intervalSeconds: 10,
  sheetUrls: ["/api/v1/media-files/f1/trickplay/0.jpg"],
};

const instantWaits = (delays: number[] = []): TrickplayLoaderWaits => ({
  delay: (milliseconds) => {
    delays.push(milliseconds);
    return Promise.resolve();
  },
  visible: () => Promise.resolve(),
});

function fakeApi(manifests: (TrickplayLayout | undefined)[]) {
  const getManifest = vi.fn<TrickplayApi["getManifest"]>(() =>
    Promise.resolve(manifests.length > 1 ? manifests.shift() : manifests[0]),
  );
  const requestGeneration = vi.fn<TrickplayApi["requestGeneration"]>(() => Promise.resolve());
  return {
    api: { getManifest, requestGeneration } satisfies TrickplayApi,
    getManifest,
    requestGeneration,
  };
}

describe("loadTrickplay", () => {
  it("takes a ready manifest without asking for generation", async () => {
    const { api, requestGeneration } = fakeApi([layout]);
    const onLayout = vi.fn();

    await loadTrickplay({
      fileId: "f1",
      api,
      waits: instantWaits(),
      signal: new AbortController().signal,
      onLayout,
    });

    expect(onLayout).toHaveBeenCalledExactlyOnceWith(layout);
    expect(requestGeneration).not.toHaveBeenCalled();
  });

  it("requests the generation once after a 404 and polls until the manifest appears", async () => {
    const { api, getManifest, requestGeneration } = fakeApi([undefined, undefined, layout]);
    const onLayout = vi.fn();
    const delays: number[] = [];

    await loadTrickplay({
      fileId: "f1",
      api,
      waits: instantWaits(delays),
      signal: new AbortController().signal,
      onLayout,
    });

    expect(requestGeneration).toHaveBeenCalledTimes(1);
    expect(getManifest).toHaveBeenCalledTimes(3);
    expect(delays).toEqual([5000, 10_000]);
    expect(onLayout).toHaveBeenCalledExactlyOnceWith(layout);
  });

  it("stops polling at the cap and shows nothing", async () => {
    const { api, getManifest } = fakeApi([undefined]);
    const onLayout = vi.fn();

    await loadTrickplay({
      fileId: "f1",
      api,
      waits: instantWaits(),
      signal: new AbortController().signal,
      onLayout,
    });

    expect(getManifest).toHaveBeenCalledTimes(trickplayPollMaxAttempts);
    expect(onLayout).not.toHaveBeenCalled();
  });

  it("ends quietly when the generation is refused", async () => {
    const { api, getManifest, requestGeneration } = fakeApi([undefined]);
    requestGeneration.mockRejectedValue(new Error("unsupported"));
    const onLayout = vi.fn();

    await expect(
      loadTrickplay({
        fileId: "f1",
        api,
        waits: instantWaits(),
        signal: new AbortController().signal,
        onLayout,
      }),
    ).resolves.toBeUndefined();

    expect(getManifest).toHaveBeenCalledTimes(1);
    expect(onLayout).not.toHaveBeenCalled();
  });

  it("ends quietly when the manifest request fails", async () => {
    const { api, getManifest } = fakeApi([layout]);
    getManifest.mockRejectedValue(new Error("boom"));
    const onLayout = vi.fn();

    await loadTrickplay({
      fileId: "f1",
      api,
      waits: instantWaits(),
      signal: new AbortController().signal,
      onLayout,
    });

    expect(onLayout).not.toHaveBeenCalled();
  });
});

describe("browserWaits", () => {
  it("rejects a delay when the signal aborts and never resolves it later", async () => {
    vi.useFakeTimers();
    const controller = new AbortController();
    const pending = browserWaits.delay(5000, controller.signal);
    const outcome = expect(pending).rejects.toMatchObject({ name: "AbortError" });
    controller.abort();
    await outcome;
    vi.useRealTimers();
  });

  it("waits for the tab to become visible", async () => {
    const hidden = vi.spyOn(document, "hidden", "get").mockReturnValue(true);
    let resolved = false;
    const pending = browserWaits.visible(new AbortController().signal).then(() => {
      resolved = true;
    });
    await Promise.resolve();
    expect(resolved).toBe(false);

    hidden.mockReturnValue(false);
    document.dispatchEvent(new Event("visibilitychange"));
    await pending;

    expect(resolved).toBe(true);
    hidden.mockRestore();
  });

  it("rejects a visibility wait when the signal aborts", async () => {
    const hidden = vi.spyOn(document, "hidden", "get").mockReturnValue(true);
    const controller = new AbortController();
    const outcome = expect(browserWaits.visible(controller.signal)).rejects.toMatchObject({
      name: "AbortError",
    });
    controller.abort();
    await outcome;
    hidden.mockRestore();
  });
});

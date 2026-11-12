import { describe, expect, it, vi } from "vitest";

import { resolveSheetUrl, SheetPreloader, type SheetLoader } from "./trickplaySheets";

const page = "http://mivo.test:8460/items/i1/play";

describe("resolveSheetUrl", () => {
  it.each([
    [
      "/api/v1/media-files/f1/trickplay/0.jpg",
      "http://mivo.test:8460/api/v1/media-files/f1/trickplay/0.jpg",
    ],
    ["http://mivo.test:8460/a.jpg", "http://mivo.test:8460/a.jpg"],
  ])("keeps the page's own origin: %s", (input, expected) => {
    expect(resolveSheetUrl(input, page)).toBe(expected);
  });

  it.each([
    "https://evil.test/0.jpg",
    "//evil.test/0.jpg",
    "http://mivo.test:9999/0.jpg",
    "javascript:alert(1)",
    "data:image/jpeg;base64,AAAA",
  ])("refuses a URL that leaves the origin: %s", (input) => {
    expect(resolveSheetUrl(input, page)).toBeUndefined();
  });

  it("refuses an unparsable page", () => {
    expect(resolveSheetUrl("/a.jpg", "not a url")).toBeUndefined();
  });
});

/** A loader whose images arrive when the test says so. */
function heldLoader() {
  const started: string[] = [];
  const settle = new Map<string, (loaded: boolean) => void>();
  const cancel = vi.fn();
  const loader: SheetLoader = (url) => {
    started.push(url);
    return {
      result: new Promise<boolean>((resolve) => {
        settle.set(url, resolve);
      }),
      cancel,
    };
  };
  return { loader, started, settle, cancel };
}

describe("SheetPreloader", () => {
  it("runs at most two sheets at once and every sheet once", async () => {
    const { loader, started, settle } = heldLoader();
    const onLoaded = vi.fn();
    const preloader = new SheetPreloader(loader, onLoaded);

    ["a", "b", "c", "a", "b"].forEach((url) => {
      preloader.request(url);
    });
    expect(started).toEqual(["a", "b"]);

    settle.get("a")?.(true);
    await vi.waitFor(() => {
      expect(started).toEqual(["a", "b", "c"]);
    });
    expect(onLoaded).toHaveBeenCalledExactlyOnceWith("a");
  });

  it("does not report or retry a failed sheet, and goes on with the queue", async () => {
    const { loader, started, settle } = heldLoader();
    const onLoaded = vi.fn();
    const preloader = new SheetPreloader(loader, onLoaded);
    ["a", "b", "c"].forEach((url) => {
      preloader.request(url);
    });

    settle.get("a")?.(false);
    await vi.waitFor(() => {
      expect(started).toEqual(["a", "b", "c"]);
    });
    preloader.request("a");

    expect(started).toEqual(["a", "b", "c"]);
    expect(onLoaded).not.toHaveBeenCalled();
  });

  it("cancels what runs on dispose and ignores later results and requests", async () => {
    const { loader, started, settle, cancel } = heldLoader();
    const onLoaded = vi.fn();
    const preloader = new SheetPreloader(loader, onLoaded);
    preloader.request("a");
    preloader.request("b");

    preloader.dispose();
    settle.get("a")?.(true);
    preloader.request("c");
    await Promise.resolve();

    expect(cancel).toHaveBeenCalledTimes(2);
    expect(started).toEqual(["a", "b"]);
    expect(onLoaded).not.toHaveBeenCalled();
  });
});

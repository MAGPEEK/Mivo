// jsdom has no Fullscreen API. This stub adds one for a test and removes it when the test ends.

import { onTestFinished } from "vitest";

/** Options of {@link stubFullscreen}: let the browser refuse requests, like without a gesture. */
export interface FakeFullscreenOptions {
  readonly refuseRequest?: boolean;
  readonly refuseExit?: boolean;
}

/** What the page asked of the Fullscreen API. */
export interface FakeFullscreen {
  /** Elements passed to `requestFullscreen`, oldest first. */
  readonly requests: readonly Element[];
  /** How often `document.exitFullscreen` was called. */
  readonly exits: () => number;
}

/** Gives jsdom a Fullscreen API that switches at once and fires `fullscreenchange`. */
export function stubFullscreen(options: FakeFullscreenOptions = {}): FakeFullscreen {
  const { refuseRequest = false, refuseExit = false } = options;
  let current: Element | null = null;
  let exits = 0;
  const requests: Element[] = [];
  const switchTo = (element: Element | null, refuse: boolean): Promise<void> => {
    if (refuse) {
      return Promise.reject(new TypeError("Permissions check failed"));
    }
    current = element;
    document.dispatchEvent(new Event("fullscreenchange"));
    return Promise.resolve();
  };
  Object.defineProperty(document, "fullscreenEnabled", { configurable: true, value: true });
  Object.defineProperty(document, "fullscreenElement", {
    configurable: true,
    get: () => current,
  });
  Object.defineProperty(document, "exitFullscreen", {
    configurable: true,
    value: () => {
      exits += 1;
      return switchTo(null, refuseExit);
    },
  });
  Object.defineProperty(HTMLElement.prototype, "requestFullscreen", {
    configurable: true,
    value(this: HTMLElement) {
      requests.push(this);
      return switchTo(this, refuseRequest);
    },
  });
  onTestFinished(() => {
    for (const name of ["fullscreenEnabled", "fullscreenElement", "exitFullscreen"]) {
      Reflect.deleteProperty(document, name);
    }
    Reflect.deleteProperty(HTMLElement.prototype, "requestFullscreen");
  });
  return { requests, exits: () => exits };
}

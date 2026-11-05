import { onTestFinished } from "vitest";

/** Controls the result of every `window.matchMedia` query in a test. */
export interface FakeMediaQueries {
  /** Changes whether the queries match and notifies their change listeners. */
  readonly setMatches: (matches: boolean) => void;
  /** How many change listeners are registered across all queries right now. */
  readonly changeListenerCount: () => number;
}

class FakeMediaQueryList extends EventTarget implements MediaQueryList {
  onchange: MediaQueryList["onchange"] = null;
  // Tracked so tests can check that a hook subscribes once and cleans up on unmount.
  readonly changeListeners = new Set<EventListenerOrEventListenerObject>();

  constructor(
    readonly media: string,
    private readonly currentMatches: () => boolean,
  ) {
    super();
  }

  get matches(): boolean {
    return this.currentMatches();
  }

  override addEventListener(
    type: string,
    listener: EventListenerOrEventListenerObject | null,
    options?: boolean | AddEventListenerOptions,
  ): void {
    if (type === "change" && listener !== null) {
      this.changeListeners.add(listener);
    }
    super.addEventListener(type, listener, options);
  }

  override removeEventListener(
    type: string,
    listener: EventListenerOrEventListenerObject | null,
    options?: boolean | EventListenerOptions,
  ): void {
    if (type === "change" && listener !== null) {
      this.changeListeners.delete(listener);
    }
    super.removeEventListener(type, listener, options);
  }

  // The legacy listener API; nothing in the app uses it, but the interface requires it.
  addListener(): void {
    throw new Error("addListener is deprecated; use addEventListener");
  }

  removeListener(): void {
    throw new Error("removeListener is deprecated; use removeEventListener");
  }
}

interface FakeMatchMedia {
  /** Notifies every list created so far after the state behind `evaluate` changed. */
  readonly notify: () => void;
  readonly changeListenerCount: () => number;
}

// Installs a fake `matchMedia` whose lists ask `evaluate` on every read.
function installFakeMatchMedia(evaluate: (query: string) => boolean): FakeMatchMedia {
  const lists: FakeMediaQueryList[] = [];
  window.matchMedia = (query: string) => {
    const list = new FakeMediaQueryList(query, () => evaluate(query));
    lists.push(list);
    return list;
  };
  onTestFinished(() => {
    Reflect.deleteProperty(window, "matchMedia");
  });
  return {
    notify: () => {
      for (const list of lists) {
        list.dispatchEvent(new Event("change"));
      }
    },
    changeListenerCount: () => lists.reduce((sum, list) => sum + list.changeListeners.size, 0),
  };
}

/**
 * Gives jsdom (which has no `matchMedia`) a fake one whose queries all match or not, e.g. to
 * simulate a narrow screen. It is removed again when the test finishes.
 */
export function stubMediaQueries(initialMatches: boolean): FakeMediaQueries {
  let matches = initialMatches;
  const { notify, changeListenerCount } = installFakeMatchMedia(() => matches);
  return {
    setMatches: (nextMatches) => {
      matches = nextMatches;
      notify();
    },
    changeListenerCount,
  };
}

/** Controls the simulated viewport width of {@link stubViewportWidth}. */
export interface FakeViewport {
  /** Changes the width in CSS pixels and notifies the change listeners of every query. */
  readonly setWidth: (widthInPixels: number) => void;
}

const pixelsPerRem = 16;

// Understands the `(width < 48rem)` form the app writes its breakpoint queries in; any other
// query does not match, so a test notices when it needs a new form.
function matchesWidth(query: string, widthInPixels: number): boolean {
  const match = /^\(width < ([\d.]+)rem\)$/.exec(query);
  if (match === null) {
    return false;
  }
  return widthInPixels < Number(match[1]) * pixelsPerRem;
}

/**
 * Gives jsdom a fake `matchMedia` that evaluates width queries against a simulated viewport,
 * e.g. to tell a phone (below `sm`) from a tablet (below `md` only). Removed when the test ends.
 */
export function stubViewportWidth(initialWidthInPixels: number): FakeViewport {
  let width = initialWidthInPixels;
  const { notify } = installFakeMatchMedia((query) => matchesWidth(query, width));
  return {
    setWidth: (nextWidth) => {
      width = nextWidth;
      notify();
    },
  };
}

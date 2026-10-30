import { onTestFinished } from "vitest";

/** Controls the result of every `window.matchMedia` query in a test. */
export interface FakeMediaQueries {
  /** Changes whether the queries match and notifies their change listeners. */
  readonly setMatches: (matches: boolean) => void;
}

class FakeMediaQueryList extends EventTarget implements MediaQueryList {
  onchange: MediaQueryList["onchange"] = null;

  constructor(
    readonly media: string,
    private readonly currentMatches: () => boolean,
  ) {
    super();
  }

  get matches(): boolean {
    return this.currentMatches();
  }

  // The legacy listener API; nothing in the app uses it, but the interface requires it.
  addListener(): void {
    throw new Error("addListener is deprecated; use addEventListener");
  }

  removeListener(): void {
    throw new Error("removeListener is deprecated; use removeEventListener");
  }
}

/**
 * Gives jsdom (which has no `matchMedia`) a fake one whose queries all match or not, e.g. to
 * simulate a narrow screen. It is removed again when the test finishes.
 */
export function stubMediaQueries(initialMatches: boolean): FakeMediaQueries {
  let matches = initialMatches;
  const lists: FakeMediaQueryList[] = [];
  window.matchMedia = (query: string) => {
    const list = new FakeMediaQueryList(query, () => matches);
    lists.push(list);
    return list;
  };
  onTestFinished(() => {
    Reflect.deleteProperty(window, "matchMedia");
  });
  return {
    setMatches: (nextMatches) => {
      matches = nextMatches;
      for (const list of lists) {
        list.dispatchEvent(new Event("change"));
      }
    },
  };
}

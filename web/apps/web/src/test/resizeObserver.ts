// jsdom has no layout and no ResizeObserver. The virtual grid sizes its columns from the width
// the observer reports, so tests install this fake: every observed element is `width` pixels
// wide until a test changes it with `setObservedWidth`.

/** Width in CSS pixels every observed element reports by default. */
export const defaultObservedWidth = 1000;

type Callback = (entries: ResizeObserverEntry[], observer: ResizeObserver) => void;

const liveObservers = new Set<FakeResizeObserver>();
let observedWidth = defaultObservedWidth;

function entryFor(target: Element): ResizeObserverEntry {
  const contentRect = { width: observedWidth, height: 0 } as DOMRectReadOnly;
  return { target, contentRect } as ResizeObserverEntry;
}

class FakeResizeObserver implements ResizeObserver {
  private readonly targets = new Set<Element>();

  constructor(private readonly callback: Callback) {
    liveObservers.add(this);
  }

  observe(target: Element): void {
    this.targets.add(target);
    this.notify();
  }

  unobserve(target: Element): void {
    this.targets.delete(target);
  }

  disconnect(): void {
    this.targets.clear();
    liveObservers.delete(this);
  }

  notify(): void {
    if (this.targets.size > 0) {
      this.callback([...this.targets].map(entryFor), this);
    }
  }
}

/** Installs the fake as the global `ResizeObserver`. */
export function installFakeResizeObserver(): void {
  globalThis.ResizeObserver = FakeResizeObserver;
}

/** Resizes every observed element, like a window resize, and tells the observers. */
export function setObservedWidth(width: number): void {
  observedWidth = width;
  liveObservers.forEach((observer) => {
    observer.notify();
  });
}

/** Back to the default width; call between tests. */
export function resetObservedWidth(): void {
  observedWidth = defaultObservedWidth;
}

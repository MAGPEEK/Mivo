/** Most sheets that load at the same time (M4-16d). */
export const maxSheetsInFlight = 2;

/**
 * Resolves a sheet URL of the manifest against the page. Only http(s) URLs of the page's own
 * origin are used: the session cookie goes along with an image request, so a manifest must not
 * be able to point it at another host. `undefined` means no preview.
 */
export function resolveSheetUrl(sheetUrl: string, pageHref: string): string | undefined {
  try {
    const page = new URL(pageHref);
    const resolved = new URL(sheetUrl, page);
    const sameOrigin = resolved.origin === page.origin && /^https?:$/.test(resolved.protocol);
    return sameOrigin ? resolved.href : undefined;
  } catch {
    return undefined;
  }
}

/** Starts loading one image; the promise says whether it arrived, the function cancels. */
export type SheetLoader = (url: string) => { result: Promise<boolean>; cancel: () => void };

/** The real loader: an `Image` that the browser caches like the later background image. */
export const imageSheetLoader: SheetLoader = (url) => {
  const image = new Image();
  const result = new Promise<boolean>((resolve) => {
    image.onload = () => {
      resolve(true);
    };
    image.onerror = () => {
      resolve(false);
    };
  });
  image.src = url;
  return {
    result,
    cancel: () => {
      image.onload = null;
      image.onerror = null;
      image.removeAttribute("src");
    },
  };
};

/**
 * Loads the sheets that are asked for, at most {@link maxSheetsInFlight} at a time and every
 * sheet once. A failed sheet is not retried: its tiles stay time only.
 */
export class SheetPreloader {
  private readonly queued: string[] = [];
  private readonly known = new Set<string>();
  private readonly running = new Map<string, () => void>();
  private disposed = false;

  constructor(
    private readonly loadSheet: SheetLoader,
    private readonly onLoaded: (url: string) => void,
  ) {}

  /** Asks for a sheet; does nothing when it is queued, running, loaded or failed already. */
  request(url: string): void {
    if (this.disposed || this.known.has(url)) {
      return;
    }
    this.known.add(url);
    this.queued.push(url);
    this.startNext();
  }

  /** Cancels everything that runs; later requests do nothing. */
  dispose(): void {
    this.disposed = true;
    this.queued.length = 0;
    for (const cancel of this.running.values()) {
      cancel();
    }
    this.running.clear();
  }

  private startNext(): void {
    while (this.running.size < maxSheetsInFlight) {
      const url = this.queued.shift();
      if (url === undefined) {
        return;
      }
      const { result, cancel } = this.loadSheet(url);
      this.running.set(url, cancel);
      void result.then((loaded) => {
        this.finish(url, loaded);
      });
    }
  }

  private finish(url: string, loaded: boolean): void {
    if (this.disposed) {
      return;
    }
    this.running.delete(url);
    if (loaded) {
      this.onLoaded(url);
    }
    this.startNext();
  }
}

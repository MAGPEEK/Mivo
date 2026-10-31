// Serialises session requests (task M2-27, ADR 0024 "seeking never trips the limit"): at most
// one `POST /api/v1/playback/sessions` is in flight per player. Each request names the session
// it replaces, and the server reuses that session's slot. Overlapping requests would all name
// the same old session, so only the first would replace it and the others would occupy slots
// of their own. While a request is in flight, only the newest wish is kept; it is sent when the
// request settles and then replaces the session that request returned.

/** Keeps the request in flight and the newest wish waiting behind it. */
export class StartQueue<Wish> {
  private busy = false;
  private waiting: Wish | undefined;

  /**
   * Returns `true` when `wish` may be sent now (the caller sends it and later calls
   * {@link finish}). Otherwise a request is in flight: `wish` replaces any older waiting wish
   * and `false` is returned.
   */
  tryBegin(wish: Wish): boolean {
    if (this.busy) {
      this.waiting = wish;
      return false;
    }
    this.busy = true;
    return true;
  }

  /**
   * Marks the request in flight as settled and returns the wish that waited behind it, if any.
   * The caller passes that wish to {@link tryBegin} when it still needs a request.
   */
  finish(): Wish | undefined {
    const next = this.waiting;
    this.busy = false;
    this.waiting = undefined;
    return next;
  }

  /** Drops the waiting wish, e.g. on close; the request in flight still has to settle. */
  clear(): void {
    this.waiting = undefined;
  }
}

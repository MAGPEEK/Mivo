// Bounds session recreation (task M2-27, M2-21 review): a session the server lost is recreated
// at the current position once. A persistent failure, for example a segment that answers 500
// every time, must not restart FFmpeg in a loop, so a second failure near the same position
// is shown as an error.

/**
 * Seconds playback must have advanced past the last recovery before another failure may be
 * recovered again. Long enough that a failure of the recovered session's first segments counts
 * as "the same position" (hls.js requests them right after the restart).
 */
export const recoveryProgressSeconds = 10;

/** Remembers where the last recovery happened and decides whether another one is allowed. */
export class RecoveryBudget {
  private lastPosition: number | undefined;

  /** Forgets the last recovery; called when the user picks a new position or retries. */
  reset(): void {
    this.lastPosition = undefined;
  }

  /**
   * Whether a failure at `position` may be recovered. A granted recovery is remembered, so the
   * next failure needs {@link recoveryProgressSeconds} of progress first.
   */
  tryRecover(position: number): boolean {
    if (this.lastPosition !== undefined && position < this.lastPosition + recoveryProgressSeconds) {
      return false;
    }
    this.lastPosition = position;
    return true;
  }
}

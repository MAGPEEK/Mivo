import { useCallback, useRef, useState } from "react";

import { useThrottleCountdown, type ThrottleCountdown } from "../../auth";
import { applyPadKey, isCompletePin, type PadKey } from "../lib/pinPad";
import type { SelectionFailure } from "../lib/selectionFailure";

/** Options of {@link usePinEntry}. */
export interface PinEntryOptions {
  /**
   * Called with the whole PIN: on the 4th digit or on "OK". `reject` reports a failed check,
   * the same as {@link PinEntry.reject}.
   */
  readonly onComplete: (pin: string, reject: (failure: SelectionFailure) => void) => void;
  /** True while a PIN is being checked; further keys wait. */
  readonly busy: boolean;
}

/** State and actions of the PIN pad (US-02). */
export interface PinEntry {
  /** Digits entered so far; the digits themselves never leave the hook. */
  readonly length: number;
  /** The last failure of the PIN check, until the next key press. */
  readonly failure: SelectionFailure | undefined;
  /** Counts wrong PINs, so the pad can restart its shake for each one. */
  readonly wrongCount: number;
  /** The lockout after too many wrong PINs, from `retryAfterSeconds`. */
  readonly lockout: ThrottleCountdown;
  /** True while keys are ignored: during a lockout or a running check. */
  readonly blocked: boolean;
  readonly press: (key: PadKey) => void;
  /** Reports a failed check: the digits are cleared, a lockout starts its countdown. */
  readonly reject: (failure: SelectionFailure) => void;
  /** Starts over for another profile. */
  readonly reset: () => void;
}

/**
 * The PIN pad's digit buffer with auto-submit on the 4th digit, the wrong-PIN state and the
 * lockout countdown. Keyboard, pointer and remote all end up in {@link PinEntry.press}.
 */
export function usePinEntry({ onComplete, busy }: PinEntryOptions): PinEntry {
  // A ref as well as state: two keys in one frame must both see the newest buffer.
  const bufferRef = useRef("");
  // `busy` arrives with the next render; a key pressed before that must not submit again.
  const checkingRef = useRef(false);
  const [length, setLength] = useState(0);
  const [failure, setFailure] = useState<SelectionFailure>();
  const [wrongCount, setWrongCount] = useState(0);
  const lockout = useThrottleCountdown();
  const blocked = busy || lockout.throttled;
  const setBuffer = useCallback((buffer: string) => {
    bufferRef.current = buffer;
    setLength(buffer.length);
  }, []);
  const reject = (rejected: SelectionFailure): void => {
    checkingRef.current = false;
    setBuffer("");
    setFailure(rejected);
    if (rejected.kind === "wrongPin") {
      setWrongCount((count) => count + 1);
    } else if (rejected.kind === "locked") {
      lockout.start(rejected.seconds);
    }
  };
  const press = (key: PadKey): void => {
    if (blocked || checkingRef.current) {
      return;
    }
    const before = bufferRef.current;
    const after = applyPadKey(before, key);
    setBuffer(after);
    setFailure(undefined);
    lockout.reset();
    const completed = key === "submit" || (typeof key === "number" && after !== before);
    if (completed && isCompletePin(after)) {
      checkingRef.current = true;
      onComplete(after, reject);
    }
  };
  const reset = (): void => {
    checkingRef.current = false;
    setBuffer("");
    setFailure(undefined);
    lockout.reset();
  };
  return { length, failure, wrongCount, lockout, blocked, press, reject, reset };
}

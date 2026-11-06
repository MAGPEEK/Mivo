import { useCallback, useEffect, useState } from "react";

import { deadlineAfter, secondsUntil } from "../lib/countdown";

const tickMilliseconds = 1000;

/** State of a throttling countdown (ADR 0032 §5). */
export interface ThrottleCountdown {
  /** Seconds until the next attempt is allowed; 0 when it is allowed. */
  readonly secondsLeft: number;
  /** Length of the current or last wait in seconds, for a one-time announcement. */
  readonly totalSeconds: number;
  /** True while the wait runs. */
  readonly throttled: boolean;
  /** True once a wait has run out, until the next one starts or {@link reset} is called. */
  readonly over: boolean;
  /** Starts a wait of `seconds`. */
  readonly start: (seconds: number) => void;
  /** Forgets a finished wait. */
  readonly reset: () => void;
}

/** Counts down the wait the server asked for with `Retry-After`. */
export function useThrottleCountdown(): ThrottleCountdown {
  const [deadline, setDeadline] = useState<number>();
  const [secondsLeft, setSecondsLeft] = useState(0);
  const [totalSeconds, setTotalSeconds] = useState(0);
  useEffect(() => {
    if (deadline === undefined) {
      return undefined;
    }
    const timer = setInterval(() => {
      const left = secondsUntil(deadline, Date.now());
      setSecondsLeft(left);
      if (left === 0) {
        clearInterval(timer);
      }
    }, tickMilliseconds);
    return () => {
      clearInterval(timer);
    };
  }, [deadline]);
  const start = useCallback((seconds: number) => {
    setDeadline(deadlineAfter(seconds, Date.now()));
    setSecondsLeft(seconds);
    setTotalSeconds(seconds);
  }, []);
  const reset = useCallback(() => {
    setDeadline(undefined);
    setSecondsLeft(0);
  }, []);
  return {
    secondsLeft,
    totalSeconds,
    throttled: secondsLeft > 0,
    over: deadline !== undefined && secondsLeft === 0,
    start,
    reset,
  };
}

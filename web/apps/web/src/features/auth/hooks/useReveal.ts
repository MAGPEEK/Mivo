import { useCallback, useState } from "react";

/** Whether a password field shows its text, and the toggle for it. */
export interface Reveal {
  readonly shown: boolean;
  readonly toggle: () => void;
}

/** The show/hide state of one password field; hidden at first. */
export function useReveal(): Reveal {
  const [shown, setShown] = useState(false);
  const toggle = useCallback(() => {
    setShown((current) => !current);
  }, []);
  return { shown, toggle };
}

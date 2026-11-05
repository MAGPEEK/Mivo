import { useEffect } from "react";

// Overlays can stack (a sheet over a dialog) and unmount in any order, so the page stays locked
// until the last lock is released and only then gets its original overflow value back.
let activeLocks = 0;
let originalOverflow = "";

function lock(): void {
  if (activeLocks === 0) {
    originalOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
  }
  activeLocks += 1;
}

function unlock(): void {
  activeLocks -= 1;
  if (activeLocks === 0) {
    document.body.style.overflow = originalOverflow;
  }
}

/** Stops the page behind a modal surface from scrolling while the calling component is mounted. */
export function useScrollLock(): void {
  useEffect(() => {
    lock();
    return unlock;
  }, []);
}

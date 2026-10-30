import { useEffect, useState } from "react";

/**
 * Returns `""` on the first render and `text` right after mount. Screen readers announce a
 * live region only when its content changes, so a status region that mounts together with
 * its text stays silent; rendering it empty first makes the text a change they read.
 */
export function useDeferredAnnouncement(text: string): string {
  const [mounted, setMounted] = useState(false);
  useEffect(() => {
    // A macrotask, so the empty region is committed and seen by assistive technology first.
    const timer = setTimeout(() => {
      setMounted(true);
    }, 0);
    return () => {
      clearTimeout(timer);
    };
  }, []);
  return mounted ? text : "";
}

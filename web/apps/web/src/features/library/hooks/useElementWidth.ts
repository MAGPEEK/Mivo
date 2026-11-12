import { useEffect, useState, type RefObject } from "react";

/** Follows the content width of the element behind `ref` with a ResizeObserver; 0 until measured. */
export function useElementWidth(ref: RefObject<HTMLElement | null>): number {
  const [width, setWidth] = useState(0);
  useEffect(() => {
    const element = ref.current;
    if (element === null) {
      return undefined;
    }
    const observer = new ResizeObserver((entries) => {
      const last = entries.at(-1);
      if (last !== undefined) {
        setWidth(last.contentRect.width);
      }
    });
    observer.observe(element);
    return () => {
      observer.disconnect();
    };
  }, [ref]);
  return width;
}

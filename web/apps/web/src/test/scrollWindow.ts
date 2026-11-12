import { act, fireEvent } from "@testing-library/react";

// jsdom does not scroll. The virtual grid follows `window.scrollY` and the scroll event, so a
// test scrolls by setting the position and sending the event.
const farBelowTheList = 1_000_000;

/** Scrolls the window to `top` pixels and tells the page. */
export function scrollWindowTo(top: number): void {
  Object.defineProperty(window, "scrollY", { value: top, configurable: true, writable: true });
  act(() => {
    fireEvent.scroll(window);
  });
}

/** Scrolls the window to the end of the page, where a virtual grid asks for its next pages. */
export function scrollWindowToEnd(): void {
  scrollWindowTo(farBelowTheList);
}

/** Puts the window back to the top; call between tests. */
export function resetWindowScroll(): void {
  Object.defineProperty(window, "scrollY", { value: 0, configurable: true, writable: true });
}

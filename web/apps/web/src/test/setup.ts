import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

// jsdom has no layout and only logs "Not implemented" for scrolling. The router resets the
// scroll position on every navigation, so a no-op keeps the test output readable.
window.scrollTo = () => undefined;

// Testing Library only cleans up automatically with Vitest globals, which this workspace does
// not enable. Persisted UI preferences (e.g. the collapsed sidebar) must not leak between tests.
afterEach(() => {
  cleanup();
  window.localStorage.clear();
});

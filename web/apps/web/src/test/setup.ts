import { cleanup, configure } from "@testing-library/react";
import { afterEach } from "vitest";

// Whole-app tests render the router, the providers and axe in jsdom. On a loaded machine
// (parallel test runs, CI) the first render alone can take longer than Testing Library's
// default of 1 s for findBy/waitFor, which made a different test fail on each run. Waiting
// longer only slows down a failing test; passing tests return as soon as the UI is there.
// vitest.config.ts keeps the test timeout above this value.
const asyncUtilTimeoutMs = 5_000;

configure({ asyncUtilTimeout: asyncUtilTimeoutMs });

// jsdom has no layout and only logs "Not implemented" for scrolling. The router resets the
// scroll position on every navigation, so a no-op keeps the test output readable.
window.scrollTo = () => undefined;

// Testing Library only cleans up automatically with Vitest globals, which this workspace does
// not enable. Persisted UI preferences (e.g. the collapsed sidebar) must not leak between tests.
afterEach(() => {
  cleanup();
  window.localStorage.clear();
});

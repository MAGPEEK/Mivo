import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { useStreamInfoOverlay } from "./useStreamInfoOverlay";

// Focus handling with a rendered overlay: components/PlayerPage.overlay.test.tsx.
describe("useStreamInfoOverlay", () => {
  it("closes without moving focus when the overlay was never rendered", () => {
    const { result } = renderHook(() => useStreamInfoOverlay());
    act(() => {
      result.current.toggle();
    });
    const focused = document.activeElement;

    act(() => {
      result.current.close();
    });

    expect(result.current.open).toBe(false);
    expect(document.activeElement).toBe(focused);
  });
});

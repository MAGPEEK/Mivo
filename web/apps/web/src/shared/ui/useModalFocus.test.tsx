import { fireEvent, render, screen } from "@testing-library/react";
import { useRef } from "react";
import { describe, expect, it } from "vitest";

import { useModalFocus } from "./useModalFocus";

function Probe() {
  const ref = useRef<HTMLDivElement>(null);
  useModalFocus(ref, () => undefined);
  return (
    <div ref={ref} role="dialog" aria-label="probe">
      <button type="button">first</button>
      <input type="text" aria-label="skipped" disabled />
      <input type="hidden" aria-label="hidden" />
      <div tabIndex={-1}>programmatic</div>
      <details>
        <summary>more</summary>
      </details>
      <div tabIndex={0} role="button">
        last
      </div>
    </div>
  );
}

describe("useModalFocus", () => {
  it("wraps Tab over summary and tabindex targets but skips disabled and tabindex -1", () => {
    render(<Probe />);
    const last = screen.getByText("last");
    last.focus();
    fireEvent.keyDown(last, { key: "Tab" });

    expect(document.activeElement).toBe(screen.getByText("first"));
    screen.getByText("first").focus();
    fireEvent.keyDown(screen.getByText("first"), { key: "Tab", shiftKey: true });

    expect(document.activeElement).toBe(last);
  });
});

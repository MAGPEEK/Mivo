import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { blockingAxeViolations } from "../../test/axe";
import { stubMediaQueries } from "../../test/mediaQuery";
import { Sheet } from "./Sheet";

function renderSheet(onClose = vi.fn()) {
  return render(
    <Sheet title="Sort" closeLabel="Close" onClose={onClose}>
      <button type="button">Title</button>
    </Sheet>,
  );
}

describe("Sheet", () => {
  it("is a labelled modal dialog centred on wide screens", () => {
    renderSheet();
    const dialog = screen.getByRole("dialog", { name: "Sort" });

    expect(dialog.getAttribute("aria-modal")).toBe("true");
    expect(dialog.getAttribute("data-placement")).toBe("center");
  });

  it("is a bottom sheet on narrow screens that skips motion for reduced-motion users", () => {
    stubMediaQueries(true);
    renderSheet();
    const dialog = screen.getByRole("dialog");

    expect(dialog.getAttribute("data-placement")).toBe("bottom");
    expect(dialog.className).toContain("motion-reduce:transition-none");
  });

  it("focuses inside, wraps Tab, closes on Escape, close button and scrim", () => {
    const onClose = vi.fn();
    renderSheet(onClose);
    const closeButton = screen.getByRole("button", { name: "Close" });

    expect(document.activeElement).toBe(closeButton);
    fireEvent.keyDown(closeButton, { key: "Tab", shiftKey: true });
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Title" }));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    fireEvent.click(closeButton);
    fireEvent.mouseDown(screen.getByTestId("sheet-scrim"));
    fireEvent.mouseDown(screen.getByRole("dialog"));

    expect(onClose).toHaveBeenCalledTimes(3);
  });

  it("locks page scrolling while open and restores it afterwards", () => {
    const { unmount } = renderSheet();
    expect(document.body.style.overflow).toBe("hidden");
    unmount();

    expect(document.body.style.overflow).toBe("");
  });

  it("has no blocking accessibility violations", async () => {
    renderSheet();

    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });
});

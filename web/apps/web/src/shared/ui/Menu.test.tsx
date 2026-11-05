import { act, fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { blockingAxeViolations } from "../../test/axe";
import { findTypeAhead, nextEnabledIndex } from "./menuNavigation";
import { Menu, type MenuEntry } from "./Menu";
import { Sheet } from "./Sheet";

function entries(onSelect: (id: string) => void): MenuEntry[] {
  return [
    {
      id: "play",
      label: "Play",
      onSelect: () => {
        onSelect("play");
      },
    },
    {
      id: "queue",
      label: "Add to queue",
      onSelect: () => {
        onSelect("queue");
      },
    },
    {
      id: "gone",
      label: "Gone",
      disabled: true,
      onSelect: () => {
        onSelect("gone");
      },
    },
    {
      id: "sort",
      label: "Sort by title",
      kind: "radio",
      checked: true,
      onSelect: () => {
        onSelect("sort");
      },
    },
    {
      id: "hide",
      label: "Hide watched",
      kind: "checkbox",
      checked: false,
      onSelect: () => {
        onSelect("hide");
      },
    },
  ];
}

function renderMenu(onSelect: (id: string) => void = () => undefined) {
  render(<Menu trigger="Actions" entries={entries(onSelect)} />);
  return screen.getByRole("button", { name: "Actions" });
}

describe("Menu", () => {
  it("opens with ArrowDown on the first item and exposes menu roles", () => {
    const trigger = renderMenu();
    fireEvent.keyDown(trigger, { key: "ArrowDown" });

    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByRole("menu")).toBeTruthy();
    expect(document.activeElement).toBe(screen.getByRole("menuitem", { name: "Play" }));
    expect(
      screen.getByRole("menuitemradio", { name: /Sort by title/ }).getAttribute("aria-checked"),
    ).toBe("true");
    expect(
      screen.getByRole("menuitemcheckbox", { name: /Hide watched/ }).getAttribute("aria-checked"),
    ).toBe("false");
  });

  it("opens on the last item with ArrowUp and navigates with wrapping, skipping disabled", () => {
    const trigger = renderMenu();
    fireEvent.keyDown(trigger, { key: "ArrowUp" });
    const menu = screen.getByRole("menu");

    expect(document.activeElement).toBe(screen.getByRole("menuitemcheckbox"));
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    expect(document.activeElement).toBe(screen.getByRole("menuitem", { name: "Play" }));
    fireEvent.keyDown(menu, { key: "ArrowUp" });
    expect(document.activeElement).toBe(screen.getByRole("menuitemcheckbox"));
    fireEvent.keyDown(menu, { key: "Home" });
    expect(document.activeElement).toBe(screen.getByRole("menuitem", { name: "Play" }));
    fireEvent.keyDown(menu, { key: "End" });
    expect(document.activeElement).toBe(screen.getByRole("menuitemcheckbox"));
    fireEvent.keyDown(menu, { key: "ArrowUp" });
    expect(document.activeElement).toBe(screen.getByRole("menuitemradio"));
  });

  it("moves focus by type-ahead", () => {
    vi.useFakeTimers();
    try {
      const trigger = renderMenu();
      fireEvent.click(trigger);
      fireEvent.keyDown(screen.getByRole("menu"), { key: "h" });

      expect(document.activeElement).toBe(screen.getByRole("menuitemcheckbox"));
      act(() => {
        vi.advanceTimersByTime(600);
      });
      fireEvent.keyDown(screen.getByRole("menu"), { key: "a" });
      expect(document.activeElement).toBe(screen.getByRole("menuitem", { name: "Add to queue" }));
    } finally {
      vi.useRealTimers();
    }
  });

  it("closes on Escape and returns focus without reaching outer handlers", () => {
    const outer = vi.fn();
    const trigger = renderMenu();
    document.body.addEventListener("keydown", outer);
    fireEvent.click(trigger);
    fireEvent.keyDown(document.activeElement ?? trigger, { key: "Escape" });
    document.body.removeEventListener("keydown", outer);

    expect(screen.queryByRole("menu")).toBeNull();
    expect(document.activeElement).toBe(trigger);
    expect(outer).not.toHaveBeenCalled();
  });

  it("selects an item, closes and returns focus", () => {
    const onSelect = vi.fn();
    const trigger = renderMenu(onSelect);
    fireEvent.click(trigger);
    fireEvent.click(screen.getByRole("menuitem", { name: "Add to queue" }));

    expect(onSelect).toHaveBeenCalledWith("queue");
    expect(screen.queryByRole("menu")).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });

  it("closes on Tab and on an outside press", () => {
    const trigger = renderMenu();
    fireEvent.click(trigger);
    fireEvent.keyDown(screen.getByRole("menu"), { key: "Tab" });
    expect(screen.queryByRole("menu")).toBeNull();

    fireEvent.click(trigger);
    fireEvent.pointerDown(screen.getByRole("menu"));
    expect(screen.getByRole("menu")).toBeTruthy();
    fireEvent.pointerDown(document.body);
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("opens upward when there is no room below", () => {
    const original = window.innerHeight;
    Object.defineProperty(window, "innerHeight", { value: 100, configurable: true });
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      top: 80,
      bottom: 90,
    } as DOMRect);
    vi.spyOn(HTMLElement.prototype, "offsetHeight", "get").mockReturnValue(120);
    try {
      fireEvent.click(renderMenu());

      expect(screen.getByRole("menu").getAttribute("data-placement")).toBe("up");
    } finally {
      vi.restoreAllMocks();
      Object.defineProperty(window, "innerHeight", { value: original, configurable: true });
    }
  });

  it("has no blocking accessibility violations when open", async () => {
    fireEvent.click(renderMenu());

    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });
});

describe("menu navigation helpers", () => {
  it("finds the next enabled index and reports none when all are disabled", () => {
    expect(nextEnabledIndex([false, true, false], 0, 1)).toBe(2);
    expect(nextEnabledIndex([true, true], 0, 1)).toBe(-1);
  });

  it.each([
    { disabled: [false, true, false], from: 0, step: 1, expected: 2 },
    { disabled: [false, true, false], from: 0, step: -1, expected: 2 },
    { disabled: [false, false, false], from: 2, step: 1, expected: 0 },
    { disabled: [false], from: 0, step: 1, expected: 0 },
    { disabled: [true, true], from: 0, step: 1, expected: -1 },
    { disabled: [], from: -1, step: 1, expected: -1 },
  ] as const)("nextEnabledIndex($disabled, $from, $step) = $expected", (row) => {
    expect(nextEnabledIndex(row.disabled, row.from, row.step)).toBe(row.expected);
  });

  const labels = ["Apple", "Avocado", "Banana", "Apricot"];
  const none = [false, false, false, false];
  it.each([
    { name: "single letter cycles forward", typed: "a", current: 0, disabled: none, expected: 1 },
    { name: "single letter wraps", typed: "a", current: 3, disabled: none, expected: 0 },
    { name: "repeated letter cycles", typed: "aa", current: 1, disabled: none, expected: 3 },
    { name: "longer input keeps a match", typed: "av", current: 1, disabled: none, expected: 1 },
    { name: "case-insensitive", typed: "B", current: 0, disabled: none, expected: 2 },
    {
      name: "skips disabled",
      typed: "a",
      current: 0,
      disabled: [false, true, false, false],
      expected: 3,
    },
    { name: "no match", typed: "z", current: 0, disabled: none, expected: -1 },
    {
      name: "longer input without a current entry starts at the top",
      typed: "ap",
      current: -1,
      disabled: none,
      expected: 0,
    },
  ])("findTypeAhead: $name", (row) => {
    expect(findTypeAhead(labels, row.disabled, row.typed, row.current)).toBe(row.expected);
  });
});

describe("Menu edge cases", () => {
  it("names the menu after its button when no aria-label is given", () => {
    fireEvent.click(renderMenu());

    expect(screen.getByRole("menu", { name: "Actions" })).toBeTruthy();
  });

  it("uses the aria-label for an icon-only button and its menu", () => {
    render(
      <Menu
        trigger={<span aria-hidden="true">⋯</span>}
        ariaLabel="More"
        entries={entries(vi.fn())}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "More" }));

    expect(screen.getByRole("menu", { name: "More" })).toBeTruthy();
  });

  it("ignores type-ahead with Alt, Ctrl or Meta held", () => {
    fireEvent.click(renderMenu());
    const menu = screen.getByRole("menu");
    fireEvent.keyDown(menu, { key: "h", altKey: true });
    fireEvent.keyDown(menu, { key: "h", ctrlKey: true });
    fireEvent.keyDown(menu, { key: "h", metaKey: true });

    expect(document.activeElement).toBe(screen.getByRole("menuitem", { name: "Play" }));
  });

  it("closes from the button when clicked again without moving focus away", () => {
    const trigger = renderMenu();
    fireEvent.click(trigger);
    fireEvent.click(trigger);

    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("closes only the menu on the first Escape inside a sheet and the sheet on the second", () => {
    const onClose = vi.fn();
    render(
      <Sheet title="Options" closeLabel="Close" onClose={onClose}>
        <Menu trigger="Actions" entries={entries(vi.fn())} />
      </Sheet>,
    );
    const trigger = screen.getByRole("button", { name: "Actions" });
    fireEvent.click(trigger);
    fireEvent.keyDown(document.activeElement ?? trigger, { key: "Escape" });

    expect(screen.queryByRole("menu")).toBeNull();
    expect(onClose).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(trigger);
    fireEvent.keyDown(trigger, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});

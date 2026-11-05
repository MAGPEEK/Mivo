import { fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../test/axe";
import { rovingTarget } from "./rovingFocus";
import { SegmentedControl } from "./SegmentedControl";
import { Tabs } from "./Tabs";

function TabsHarness({ orientation }: { orientation?: "horizontal" | "vertical" }) {
  const [value, setValue] = useState("movies");
  return (
    <Tabs
      label="Library"
      {...(orientation === undefined ? {} : { orientation })}
      value={value}
      onChange={setValue}
      tabs={[
        { id: "movies", label: "Movies" },
        { id: "shows", label: "Shows" },
        { id: "music", label: "Music" },
      ]}
    >
      <p>Panel {value}</p>
    </Tabs>
  );
}

function SegmentHarness({ initial = "grid" }: { initial?: string }) {
  const [value, setValue] = useState(initial);
  return (
    <SegmentedControl
      label="View style"
      value={value}
      onChange={setValue}
      options={[
        { id: "grid", label: "Grid" },
        { id: "list", label: "List" },
      ]}
    />
  );
}

describe("rovingTarget", () => {
  it.each([
    { key: "ArrowRight", current: 0, count: 3, orientation: "horizontal", expected: 1 },
    { key: "ArrowLeft", current: 0, count: 3, orientation: "horizontal", expected: 2 },
    { key: "ArrowDown", current: 0, count: 3, orientation: "horizontal", expected: undefined },
    { key: "ArrowUp", current: 0, count: 3, orientation: "horizontal", expected: undefined },
    { key: "ArrowDown", current: 2, count: 3, orientation: "vertical", expected: 0 },
    { key: "ArrowRight", current: 0, count: 3, orientation: "vertical", expected: undefined },
    { key: "ArrowUp", current: 1, count: 3, orientation: "both", expected: 0 },
    { key: "ArrowRight", current: 0, count: 1, orientation: "both", expected: 0 },
    { key: "Home", current: 2, count: 3, orientation: "horizontal", expected: 0 },
    { key: "End", current: 0, count: 3, orientation: "vertical", expected: 2 },
    { key: "a", current: 0, count: 3, orientation: "both", expected: undefined },
  ] as const)("$orientation $key from $current of $count -> $expected", (row) => {
    expect(rovingTarget(row.key, row.current, row.count, row.orientation)).toBe(row.expected);
  });
});

describe("Tabs", () => {
  it("ignores Up/Down when horizontal and uses them when vertical", () => {
    const { unmount } = render(<TabsHarness />);
    const movies = screen.getByRole("tab", { name: "Movies" });
    movies.focus();
    fireEvent.keyDown(movies, { key: "ArrowDown" });
    expect(movies.getAttribute("aria-selected")).toBe("true");
    unmount();

    render(<TabsHarness orientation="vertical" />);
    expect(screen.getByRole("tablist").getAttribute("aria-orientation")).toBe("vertical");
    const verticalMovies = screen.getByRole("tab", { name: "Movies" });
    verticalMovies.focus();
    fireEvent.keyDown(verticalMovies, { key: "ArrowRight" });
    expect(verticalMovies.getAttribute("aria-selected")).toBe("true");
    fireEvent.keyDown(verticalMovies, { key: "ArrowDown" });
    expect(screen.getByRole("tab", { name: "Shows" }).getAttribute("aria-selected")).toBe("true");
  });

  it("marks the selected tab and keeps only it in the Tab order", () => {
    render(<TabsHarness />);

    expect(screen.getByRole("tab", { name: "Movies" }).getAttribute("aria-selected")).toBe("true");
    expect(screen.getByRole("tab", { name: "Movies" }).tabIndex).toBe(0);
    expect(screen.getByRole("tab", { name: "Shows" }).tabIndex).toBe(-1);
    expect(screen.getByRole("tabpanel", { name: "Movies" }).textContent).toBe("Panel movies");
  });

  it("moves focus and selection with arrows, Home and End, wrapping", () => {
    render(<TabsHarness />);
    const movies = screen.getByRole("tab", { name: "Movies" });
    movies.focus();

    fireEvent.keyDown(movies, { key: "ArrowRight" });
    expect(document.activeElement).toBe(screen.getByRole("tab", { name: "Shows" }));
    expect(screen.getByRole("tab", { name: "Shows" }).getAttribute("aria-selected")).toBe("true");
    fireEvent.keyDown(document.activeElement ?? movies, { key: "End" });
    expect(screen.getByRole("tab", { name: "Music" }).getAttribute("aria-selected")).toBe("true");
    fireEvent.keyDown(document.activeElement ?? movies, { key: "ArrowRight" });
    expect(screen.getByRole("tab", { name: "Movies" }).getAttribute("aria-selected")).toBe("true");
    fireEvent.keyDown(document.activeElement ?? movies, { key: "ArrowLeft" });
    expect(screen.getByRole("tab", { name: "Music" }).getAttribute("aria-selected")).toBe("true");
    fireEvent.keyDown(document.activeElement ?? movies, { key: "Home" });
    expect(screen.getByRole("tab", { name: "Movies" }).getAttribute("aria-selected")).toBe("true");
  });

  it("selects on click and ignores other keys", () => {
    render(<TabsHarness />);
    fireEvent.click(screen.getByRole("tab", { name: "Music" }));
    fireEvent.keyDown(screen.getByRole("tab", { name: "Music" }), { key: "a" });

    expect(screen.getByRole("tabpanel").textContent).toBe("Panel music");
  });

  it("has no blocking accessibility violations", async () => {
    render(<TabsHarness />);

    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });
});

describe("SegmentedControl", () => {
  it("checks nothing for an unknown value but keeps the first option tabbable", () => {
    render(<SegmentHarness initial="poster" />);

    expect(screen.getByRole("radio", { name: "Grid" }).getAttribute("aria-checked")).toBe("false");
    expect(screen.getByRole("radio", { name: "List" }).getAttribute("aria-checked")).toBe("false");
    expect(screen.getByRole("radio", { name: "Grid" }).tabIndex).toBe(0);
  });

  it("accepts Up/Down as well as Left/Right", () => {
    render(<SegmentHarness />);
    const grid = screen.getByRole("radio", { name: "Grid" });
    grid.focus();
    fireEvent.keyDown(grid, { key: "ArrowDown" });

    expect(screen.getByRole("radio", { name: "List" }).getAttribute("aria-checked")).toBe("true");
  });

  it("is a radio group whose arrows select the neighbour", () => {
    render(<SegmentHarness />);
    const grid = screen.getByRole("radio", { name: "Grid" });
    grid.focus();

    expect(screen.getByRole("radiogroup", { name: "View style" })).toBeTruthy();
    expect(grid.getAttribute("aria-checked")).toBe("true");
    fireEvent.keyDown(grid, { key: "ArrowRight" });
    expect(screen.getByRole("radio", { name: "List" }).getAttribute("aria-checked")).toBe("true");
    expect(document.activeElement).toBe(screen.getByRole("radio", { name: "List" }));
    expect(grid.tabIndex).toBe(-1);
  });

  it("selects on click and has no blocking accessibility violations", async () => {
    render(<SegmentHarness />);
    fireEvent.click(screen.getByRole("radio", { name: "List" }));

    expect(screen.getByRole("radio", { name: "List" }).getAttribute("aria-checked")).toBe("true");
    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });
});

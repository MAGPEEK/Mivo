import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { CountBadge, ProgressBar, WatchedBadge } from "./WatchBadges";

describe("ProgressBar", () => {
  it.each([
    [42, "42"],
    [-5, "0"],
    [140, "100"],
  ])("reports %d percent as %s", (percent, expected) => {
    render(<ProgressBar percent={percent} label="Watched" />);

    const bar = screen.getByRole("progressbar", { name: "Watched" });
    expect(bar.getAttribute("aria-valuenow")).toBe(expected);
    expect(bar.getAttribute("aria-valuemin")).toBe("0");
    expect(bar.getAttribute("aria-valuemax")).toBe("100");
    expect(bar.querySelector("div")?.style.width).toBe(`${expected}%`);
  });

  it("takes another position than the bottom edge", () => {
    render(<ProgressBar percent={10} label="Watched" className="mt-1 rounded-sm" />);

    const bar = screen.getByRole("progressbar");
    expect(bar.className).toContain("mt-1");
    expect(bar.className).not.toContain("absolute");
  });
});

describe("badges", () => {
  it("names the watched corner", () => {
    render(<WatchedBadge label="Watched" />);

    expect(screen.getByRole("img", { name: "Watched" })).toBeTruthy();
  });

  it("shows the count and names it", () => {
    render(<CountBadge count={14} label="14 unwatched episodes" />);

    expect(screen.getByRole("img", { name: "14 unwatched episodes" }).textContent).toBe("14");
  });
});

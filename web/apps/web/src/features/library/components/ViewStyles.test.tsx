import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nextProvider } from "react-i18next";
import { describe, expect, it, vi } from "vitest";

import { createI18n } from "../../../shared/i18n";
import { blockingAxeViolations } from "../../../test/axe";
import { fakeItem, fakePoster, fakeShow } from "../../../test/libraryFixtures";
import { libraryLocales, libraryNamespace } from "../locales";
import { DetailsView, ListView } from "./ItemRows";
import { SizeSlider } from "./SizeSlider";
import { ViewControls } from "./ViewControls";
import { useViewPreference } from "../hooks/useViewPreference";

async function renderInRouter(content: ReactNode, language = "en"): Promise<HTMLElement> {
  const rootRoute = createRootRoute({ component: () => <main>{content}</main> });
  const router = createRouter({
    routeTree: rootRoute,
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  const { container } = render(
    <I18nextProvider i18n={createI18n([language], { [libraryNamespace]: libraryLocales })}>
      <RouterProvider router={router} />
    </I18nextProvider>,
  );
  await screen.findByRole("main");
  return container;
}

const movie = fakeItem({ title: "Big Buck Bunny", year: 2008, poster: fakePoster });
const show = fakeShow();

describe("ListView", () => {
  it("shows title, year, duration and the resolution chip per row", async () => {
    const container = await renderInRouter(<ListView items={[movie, show]} label="Videos" />);

    expect(screen.getByRole("list", { name: "Videos" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Big Buck Bunny" })).toBeTruthy();
    expect(screen.getByText("2008")).toBeTruthy();
    expect(screen.getByText("9:56")).toBeTruthy();
    expect(screen.getByText("1080p")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Big Buck Bunny Show" })).toBeTruthy();
    expect(await blockingAxeViolations(container)).toEqual([]);
  });

  it("focuses the link of the requested item", async () => {
    await renderInRouter(<ListView items={[movie, show]} label="Videos" focusItemId={show.id} />);
    expect(document.activeElement).toBe(screen.getByRole("link", { name: show.title }));
  });
});

describe("unavailable items in the row views", () => {
  const broken = fakeItem({ id: "broken", title: "Broken", media: { available: false } });

  it("marks an unavailable item in List with a badge and a muted title", async () => {
    await renderInRouter(<ListView items={[broken, movie]} label="Videos" />);

    expect(screen.getByText("Unavailable")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Broken" }).className).toContain("text-text-muted");
    expect(screen.getByRole("link", { name: "Big Buck Bunny" }).className).not.toContain(
      "text-text-muted",
    );
  });

  it("marks an unavailable item in Details with the alert icon", async () => {
    await renderInRouter(<DetailsView items={[broken]} label="Videos" />);

    const badge = screen.getByText("Unavailable").parentElement;
    expect(badge?.querySelector("svg")).not.toBeNull();
  });

  it("keeps the list role in both views, which Safari drops from unstyled lists", async () => {
    const container = await renderInRouter(
      <>
        <ListView items={[movie]} label="Rows" />
        <DetailsView items={[movie]} label="Thumbnails" />
      </>,
    );

    const lists = [...container.querySelectorAll("ul")];
    expect(lists.map((list) => list.getAttribute("role"))).toEqual(["list", "list"]);
  });
});

describe("DetailsView", () => {
  it("shows a thumbnail, the summary line and technical chips", async () => {
    const container = await renderInRouter(<DetailsView items={[movie, show]} label="Videos" />);

    expect(screen.getByRole("heading", { level: 2, name: "Big Buck Bunny" })).toBeTruthy();
    expect(screen.getByText("2008 · 9:56")).toBeTruthy();
    expect(screen.getByText("MKV · H.264 · AAC")).toBeTruthy();
    expect(screen.getByText("1080p")).toBeTruthy();
    expect(screen.getByText("2015 · 3 seasons")).toBeTruthy();
    expect(document.querySelector("img")?.getAttribute("sizes")).toBe("72px");
    expect(await blockingAxeViolations(container)).toEqual([]);
  });
});

describe("SizeSlider", () => {
  it("has five steps and reports the chosen one", async () => {
    const onChange = vi.fn();
    await renderInRouter(<SizeSlider step={2} onChange={onChange} />);

    const slider = screen.getByRole("slider", { name: "Card size" });
    expect(slider.getAttribute("min")).toBe("0");
    expect(slider.getAttribute("max")).toBe("4");
    expect(slider.getAttribute("step")).toBe("1");
    expect(slider.getAttribute("aria-valuetext")).toBe("Step 3 of 5, 160 pixels");
    fireEvent.change(slider, { target: { value: "4" } });
    expect(onChange).toHaveBeenCalledWith(4);
  });
});

function ControlsHarness() {
  const view = useViewPreference(undefined);
  return <ViewControls view={view} />;
}

describe("ViewControls", () => {
  it("switches the view and shows the slider only for the grid", async () => {
    await renderInRouter(<ControlsHarness />);

    expect(screen.getByRole("radiogroup", { name: "View style" })).toBeTruthy();
    expect(screen.getByRole("radio", { name: "Grid" }).getAttribute("aria-checked")).toBe("true");
    expect(screen.getByRole("slider", { name: "Card size" })).toBeTruthy();

    fireEvent.click(screen.getByRole("radio", { name: "List" }));
    expect(screen.getByRole("radio", { name: "List" }).getAttribute("aria-checked")).toBe("true");
    expect(screen.queryByRole("slider")).toBeNull();
  });

  it("is translated to German", async () => {
    await renderInRouter(<ControlsHarness />, "de");
    expect(screen.getByRole("radio", { name: "Raster" })).toBeTruthy();
    expect(screen.getByRole("slider", { name: "Kartengröße" })).toBeTruthy();
  });
});

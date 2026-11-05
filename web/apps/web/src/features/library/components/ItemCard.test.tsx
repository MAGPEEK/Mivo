import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nextProvider } from "react-i18next";
import { describe, expect, it } from "vitest";

import { createI18n } from "../../../shared/i18n";
import { blockingAxeViolations } from "../../../test/axe";
import { fakeItem, fakePoster } from "../../../test/libraryFixtures";
import { libraryLocales, libraryNamespace } from "../locales";
import { ItemCard } from "./ItemCard";

const still = { id: "still-1", tag: "tag-s" };

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

function imageSource(): string | null {
  return document.querySelector("img")?.getAttribute("src") ?? null;
}

describe("ItemCard formats", () => {
  it("shows a poster card in a page grid with an h2 title", async () => {
    await renderInRouter(<ItemCard item={fakeItem({ poster: fakePoster, thumb: still })} />);

    expect(screen.getByRole("heading", { level: 2, name: "Big Buck Bunny" })).toBeTruthy();
    expect(imageSource()).toContain(fakePoster.id);
  });

  it("shows the still and the episode code on an episode card in a row", async () => {
    const episode = fakeItem({ kind: "episode", title: "Pilot", number: 9, thumb: still });
    await renderInRouter(
      <ItemCard
        item={episode}
        format="episode"
        seasonNumber={3}
        rowCard={{ tabIndex: -1, onKeyDown: () => undefined, onFocus: () => undefined }}
      />,
    );

    const link = screen.getByRole("link", { name: "Pilot" });
    expect(link.tabIndex).toBe(-1);
    expect(screen.getByRole("heading", { level: 3, name: "Pilot" })).toBeTruthy();
    expect(screen.getByText("S3 · E9")).toBeTruthy();
    expect(imageSource()).toContain(still.id);
    expect(document.querySelector("img")?.getAttribute("sizes")).toBe("256px");
  });

  it("formats the episode code in German", async () => {
    const episode = fakeItem({ kind: "episode", title: "Pilot", number: 1, numberEnd: 2 });
    await renderInRouter(<ItemCard item={episode} format="episode" seasonNumber={1} />, "de");

    expect(screen.getByText("S1 · E1–E2")).toBeTruthy();
  });

  it("leaves out the episode code while the season is unknown", async () => {
    const episode = fakeItem({ kind: "episode", title: "Pilot", number: 9 });
    await renderInRouter(<ItemCard item={episode} format="episode" />);

    expect(screen.queryByText(/E9/)).toBeNull();
  });

  it("draws a square card and keeps the overlay slot accessible", async () => {
    const container = await renderInRouter(
      <ItemCard
        item={fakeItem({ poster: fakePoster })}
        format="square"
        overlay={<span role="img" aria-label="Watched" />}
      />,
    );

    expect(container.querySelector(".aspect-square")).not.toBeNull();
    expect(screen.getByRole("img", { name: "Watched" })).toBeTruthy();
    expect(await blockingAxeViolations(container)).toEqual([]);
  });
});

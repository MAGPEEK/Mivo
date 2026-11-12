import type { ViewerState } from "@mivo/api-client";
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { fireEvent, render, screen, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nextProvider } from "react-i18next";
import { describe, expect, it, vi } from "vitest";

import { createI18n } from "../../../shared/i18n";
import { blockingAxeViolations } from "../../../test/axe";
import { fakeItem } from "../../../test/libraryFixtures";
import type { WatchedToggle } from "../hooks/useWatchedToggle";
import { libraryLocales, libraryNamespace } from "../locales";
import { ItemCard } from "./ItemCard";
import { RowItemCard } from "./RowItemCard";

const rowCard = { tabIndex: 0, onKeyDown: () => undefined, onFocus: () => undefined } as const;

function state(fields: Partial<ViewerState> = {}): ViewerState {
  return { watched: false, positionSeconds: 0, playCount: 0, ...fields };
}

function fakeToggle(fields: Partial<WatchedToggle> = {}): WatchedToggle {
  return {
    available: true,
    watched: false,
    viewerState: undefined,
    pending: false,
    confirmUnmark: false,
    failure: "none",
    toggle: () => undefined,
    ...fields,
  };
}

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

function badgeRoles(): string[] {
  const overlay = document.querySelector("[data-card-overlay]");
  return Array.from(overlay?.querySelectorAll("[role]") ?? [], (badge) => badge.role ?? "");
}

describe("watch badges on cards", () => {
  it.each(["movie", "episode", "show", "season", "album"] as const)(
    "shows no badge on a %s card without viewer state",
    async (kind) => {
      await renderInRouter(<ItemCard item={fakeItem({ kind })} />);

      expect(badgeRoles()).toEqual([]);
    },
  );

  it("shows the watched corner on a watched movie", async () => {
    const container = await renderInRouter(
      <ItemCard item={fakeItem({ viewerState: state({ watched: true, playCount: 1 }) })} />,
    );

    expect(screen.getByRole("img", { name: "Watched" })).toBeTruthy();
    expect(badgeRoles()).toEqual(["img"]);
    expect(await blockingAxeViolations(container)).toEqual([]);
  });

  it.each([
    ["poster", "movie"],
    ["episode", "episode"],
  ] as const)("shows the progress bar on a %s card", async (format, kind) => {
    // The fixture file is 596 s long.
    const item = fakeItem({ kind, viewerState: state({ positionSeconds: 149 }) });
    const container = await renderInRouter(<ItemCard item={item} format={format} />);

    const bar = screen.getByRole("progressbar", { name: "Watched 25 %" });
    expect(bar.getAttribute("aria-valuenow")).toBe("25");
    expect(await blockingAxeViolations(container)).toEqual([]);
  });

  it.each(["show", "season"] as const)(
    "shows the unwatched episode count on a %s card",
    async (kind) => {
      const item = fakeItem({ kind, viewerState: state({ unwatchedCount: 4 }) });
      await renderInRouter(<ItemCard item={item} />);

      expect(screen.getByRole("img", { name: "4 unwatched episodes" }).textContent).toBe("4");
      expect(badgeRoles()).toEqual(["img"]);
    },
  );

  it("names the badges in German", async () => {
    const item = fakeItem({ kind: "show", viewerState: state({ unwatchedCount: 1 }) });
    await renderInRouter(<ItemCard item={item} />, "de");

    expect(screen.getByRole("img", { name: "1 ungesehene Folge" })).toBeTruthy();
  });

  it("shows progress in a row and leaves the corner to the new-episodes count", async () => {
    const episode = fakeItem({
      kind: "season",
      newEpisodeCount: 3,
      viewerState: state({ unwatchedCount: 5 }),
    });
    const movie = fakeItem({ viewerState: state({ watched: true, positionSeconds: 298 }) });
    await renderInRouter(
      <>
        <RowItemCard item={episode} format="poster" rowCard={rowCard} />
        <RowItemCard item={movie} format="poster" rowCard={rowCard} />
      </>,
    );

    expect(screen.queryByRole("img", { name: "5 unwatched episodes" })).toBeNull();
    expect(screen.getByRole("img", { name: "Watched" })).toBeTruthy();
    expect(screen.getByRole("progressbar", { name: "Watched 50 %" })).toBeTruthy();
  });
});

describe("watched toggle on a card", () => {
  it("has no toggle without one, e.g. in a row", async () => {
    await renderInRouter(<ItemCard item={fakeItem()} />);

    expect(screen.queryByRole("button")).toBeNull();
  });

  it("marks the item watched from the card and comes after the link", async () => {
    const toggle = vi.fn();
    const container = await renderInRouter(
      <ItemCard item={fakeItem()} watchedToggle={fakeToggle({ toggle })} />,
    );

    const button = screen.getByRole("button", { name: "Mark Big Buck Bunny as watched" });
    const link = screen.getByRole("link", { name: "Big Buck Bunny" });
    expect(link.compareDocumentPosition(button)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
    fireEvent.click(button);

    expect(toggle).toHaveBeenCalledTimes(1);
    expect(await blockingAxeViolations(container)).toEqual([]);
  });

  it("shows the toggle's expected state in the badges, not the item's", async () => {
    const item = fakeItem({ viewerState: state({ positionSeconds: 149 }) });
    const expected = state({ watched: true });
    await renderInRouter(
      <ItemCard
        item={item}
        watchedToggle={fakeToggle({ watched: true, viewerState: expected, pending: true })}
      />,
    );

    const button = screen.getByRole("button", { name: "Mark Big Buck Bunny as unwatched" });
    expect(button.getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByRole("img", { name: "Watched" })).toBeTruthy();
    expect(screen.queryByRole("progressbar")).toBeNull();
  });

  it("announces a failed request", async () => {
    await renderInRouter(
      <ItemCard item={fakeItem()} watchedToggle={fakeToggle({ failure: "failed" })} />,
    );

    expect(screen.getByRole("status").textContent).toBe("The watched state could not be saved.");
  });

  it.each([
    ["the server cannot store watch state", fakeToggle({ failure: "unsupported" })],
    ["the kind has no watch state", fakeToggle({ available: false })],
  ])("hides the toggle when %s", async (_name, watchedToggle) => {
    await renderInRouter(<ItemCard item={fakeItem()} watchedToggle={watchedToggle} />);

    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("names the toggle in German", async () => {
    await renderInRouter(<ItemCard item={fakeItem()} watchedToggle={fakeToggle()} />, "de");

    expect(
      screen.getByRole("button", { name: "Big Buck Bunny als gesehen markieren" }),
    ).toBeTruthy();
  });
});

describe("unmarking a show or season from a card", () => {
  const watchedShow = fakeToggle({ watched: true, confirmUnmark: true });

  it("asks first and unmarks only after the confirmation", async () => {
    const toggle = vi.fn();
    await renderInRouter(<ItemCard item={fakeItem()} watchedToggle={{ ...watchedShow, toggle }} />);

    fireEvent.click(screen.getByRole("button", { name: "Mark Big Buck Bunny as unwatched" }));

    const dialog = screen.getByRole("dialog", { name: "Mark Big Buck Bunny as unwatched?" });
    expect(dialog.textContent).toContain("resume positions are cleared");
    expect(toggle).not.toHaveBeenCalled();
    fireEvent.click(within(dialog).getByRole("button", { name: "Mark as unwatched" }));
    expect(toggle).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("keeps everything as it was on cancel", async () => {
    const toggle = vi.fn();
    await renderInRouter(<ItemCard item={fakeItem()} watchedToggle={{ ...watchedShow, toggle }} />);

    fireEvent.click(screen.getByRole("button", { name: "Mark Big Buck Bunny as unwatched" }));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(toggle).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it.each([
    ["Cancel", (): void => void fireEvent.click(screen.getByRole("button", { name: "Cancel" }))],
    ["Escape", (): void => void fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" })],
    [
      "Confirm",
      (): void =>
        void fireEvent.click(
          within(screen.getByRole("dialog")).getByRole("button", { name: "Mark as unwatched" }),
        ),
    ],
  ])(
    "keeps the card's buttons shown and returns focus to the toggle after %s",
    async (_name, close) => {
      await renderInRouter(<ItemCard item={fakeItem()} watchedToggle={watchedShow} />);
      const opener = screen.getByRole("button", { name: "Mark Big Buck Bunny as unwatched" });
      opener.focus();

      fireEvent.click(opener);

      // The card's buttons hide unless hovered or focused; the dialog holds the focus.
      expect(document.querySelector("[data-card-actions] [data-keep-open]")).not.toBeNull();
      close();
      expect(screen.queryByRole("dialog")).toBeNull();
      expect(document.activeElement).toBe(opener);
      expect(document.querySelector("[data-keep-open]")).toBeNull();
    },
  );

  it("marking watched needs no confirmation", async () => {
    const toggle = vi.fn();
    await renderInRouter(<ItemCard item={fakeItem()} watchedToggle={fakeToggle({ toggle })} />);

    fireEvent.click(screen.getByRole("button", { name: "Mark Big Buck Bunny as watched" }));

    expect(toggle).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

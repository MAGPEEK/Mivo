import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { I18nextProvider } from "react-i18next";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { blockingAxeViolations } from "../../test/axe";
import { stubMediaQueries } from "../../test/mediaQuery";
import { createI18n } from "../i18n";
import { cardStride, rowGap, type CardFormat } from "../lib/rowScroll";
import { MediaRow, type SeeAllLinkProps } from "./MediaRow";

interface Movie {
  readonly id: string;
  readonly title: string;
}

function movies(count: number): Movie[] {
  return Array.from({ length: count }, (_, index) => ({
    id: `movie-${String(index)}`,
    title: `Movie ${String(index)}`,
  }));
}

// jsdom has no layout: the row's scroll container reports the faked width, its content the
// width all cards would take, like a browser does.
const rowWidth = 800;
let contentWidth = 0;

function fakeLayout(count: number, format: CardFormat = "poster"): void {
  contentWidth = count * cardStride(format) - rowGap;
}

function isScroller(element: HTMLElement): boolean {
  return element.hasAttribute("data-row-scroller");
}

const scrollTo = vi.fn<(options: ScrollToOptions) => void>();

beforeEach(() => {
  contentWidth = 0;
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockImplementation(function (
    this: HTMLElement,
  ) {
    return isScroller(this) ? rowWidth : 0;
  });
  vi.spyOn(HTMLElement.prototype, "scrollWidth", "get").mockImplementation(function (
    this: HTMLElement,
  ) {
    return isScroller(this) ? Math.max(rowWidth, contentWidth) : 0;
  });
  Element.prototype.scrollTo = scrollTo as unknown as Element["scrollTo"];
});

afterEach(() => {
  vi.restoreAllMocks();
  scrollTo.mockReset();
  Reflect.deleteProperty(Element.prototype, "scrollTo");
});

interface RenderRowOptions {
  readonly count: number;
  readonly withSeeAll?: boolean;
  readonly format?: CardFormat;
}

const i18n = createI18n(["en"]);

function rowElement({ count, withSeeAll = true, format = "poster" }: RenderRowOptions) {
  fakeLayout(count, format);
  const seeAll = withSeeAll
    ? {
        renderSeeAll: (link: SeeAllLinkProps) => (
          <a href="#all" className={link.className} aria-label={link["aria-label"]}>
            {link.children}
          </a>
        ),
      }
    : {};
  return (
    <I18nextProvider i18n={i18n}>
      <main>
        <MediaRow
          title="Recently added"
          items={movies(count)}
          format={format}
          getKey={(movie) => movie.id}
          renderCard={(movie, card) => (
            <a href={`#${movie.id}`} {...card}>
              {movie.title}
            </a>
          )}
          {...seeAll}
        />
      </main>
    </I18nextProvider>
  );
}

function renderRow(options: RenderRowOptions) {
  return render(rowElement(options));
}

// The section carries the row's name; its list needs none of its own.
function rowList(): HTMLElement {
  return within(screen.getByRole("region", { name: "Recently added" })).getByRole("list");
}

function cards(): HTMLElement[] {
  return within(rowList()).getAllByRole("link");
}

function cardAt(index: number): HTMLElement {
  const card = cards().at(index);
  if (card === undefined) {
    throw new Error(`row has no card ${String(index)}`);
  }
  return card;
}

function scroller(): HTMLElement {
  const list = rowList();
  const element = list.closest<HTMLElement>("[data-row-scroller]");
  if (element === null) {
    throw new Error("row has no scroll container");
  }
  return element;
}

function scrollRowTo(left: number): void {
  act(() => {
    scroller().scrollLeft = left;
    fireEvent.scroll(scroller());
  });
}

describe("MediaRow", () => {
  it("names the section after its title and links to all items", () => {
    renderRow({ count: 5 });

    expect(screen.getByRole("region", { name: "Recently added" })).toBeTruthy();
    expect(screen.getByRole("heading", { level: 2, name: "Recently added" })).toBeTruthy();
    const seeAll = screen.getByRole("link", { name: "See all: Recently added" });
    expect(seeAll.textContent).toContain("See all");
  });

  it("leaves out the see-all link when there is no target", () => {
    renderRow({ count: 5, withSeeAll: false });

    expect(screen.queryByRole("link", { name: /See all/ })).toBeNull();
  });

  it("renders every card of a short row", () => {
    renderRow({ count: 5 });

    expect(cards().map((card) => card.textContent)).toEqual([
      "Movie 0",
      "Movie 1",
      "Movie 2",
      "Movie 3",
      "Movie 4",
    ]);
  });

  it("renders only the visible cards of a long row", () => {
    renderRow({ count: 200 });

    const rendered = cards();
    expect(rendered.length).toBeGreaterThan(Math.floor(rowWidth / cardStride("poster")));
    expect(rendered.length).toBeLessThan(20);
    expect(rendered[0]?.textContent).toBe("Movie 0");
    const items = within(rowList()).getAllByRole("listitem");
    expect(items[0]?.getAttribute("aria-setsize")).toBe("200");
    expect(items[0]?.getAttribute("aria-posinset")).toBe("1");
  });

  it("follows scrolling in a long row", () => {
    renderRow({ count: 200 });
    scrollRowTo(cardStride("poster") * 100);

    const titles = cards().map((card) => card.textContent);
    expect(titles).toContain("Movie 100");
    // The card that holds the tab stop stays mounted, so Tab still reaches the row.
    expect(titles).toContain("Movie 0");
    expect(titles).not.toContain("Movie 50");
  });

  it("moves the cards of a mounted long row to the offsets of a new format", () => {
    const { rerender } = renderRow({ count: 200 });
    const secondCardOffset = () =>
      within(rowList()).getAllByRole("listitem")[1]?.style.transform ?? "";
    expect(secondCardOffset()).toBe(`translateX(${String(cardStride("poster"))}px)`);

    rerender(rowElement({ count: 200, format: "episode" }));

    expect(secondCardOffset()).toBe(`translateX(${String(cardStride("episode"))}px)`);
  });

  it("makes one card tabbable and moves focus with the arrow keys", () => {
    renderRow({ count: 5 });
    const first = cardAt(0);
    const second = cardAt(1);
    expect(cards().map((card) => card.tabIndex)).toEqual([0, -1, -1, -1, -1]);

    first.focus();
    fireEvent.keyDown(first, { key: "ArrowRight" });

    expect(document.activeElement).toBe(second);
    expect(cards().map((card) => card.tabIndex)).toEqual([-1, 0, -1, -1, -1]);
    fireEvent.keyDown(second, { key: "ArrowLeft" });
    expect(document.activeElement).toBe(first);
    fireEvent.keyDown(first, { key: "ArrowLeft" });
    expect(document.activeElement).toBe(first);
  });

  it("jumps to the ends with Home and End, also to cards not rendered yet", () => {
    renderRow({ count: 200 });
    const first = cardAt(0);
    first.focus();

    fireEvent.keyDown(first, { key: "End" });

    expect(document.activeElement?.textContent).toBe("Movie 199");
    fireEvent.keyDown(document.activeElement as HTMLElement, { key: "Home" });
    expect(document.activeElement?.textContent).toBe("Movie 0");
  });

  it("moves the tab stop to a card that gets focus by click or Tab", () => {
    renderRow({ count: 5 });
    const third = cardAt(2);

    fireEvent.focus(third);

    expect(third.tabIndex).toBe(0);
  });

  it("disables the arrow buttons at the ends of the row", () => {
    renderRow({ count: 20 });
    const previous = screen.getByRole("button", { name: "Scroll Recently added back" });
    const next = screen.getByRole("button", { name: "Scroll Recently added forward" });
    expect(previous.getAttribute("aria-disabled")).toBe("true");
    expect(next.getAttribute("aria-disabled")).toBe("false");

    scrollRowTo(contentWidth - rowWidth);

    expect(previous.getAttribute("aria-disabled")).toBe("false");
    expect(next.getAttribute("aria-disabled")).toBe("true");
  });

  it("keeps focus on the next button when the row reaches its end", () => {
    renderRow({ count: 20 });
    const next = screen.getByRole("button", { name: "Scroll Recently added forward" });
    next.focus();

    scrollRowTo(contentWidth - rowWidth);

    expect(document.activeElement).toBe(next);
    fireEvent.click(next);
    expect(scrollTo).not.toHaveBeenCalled();
  });

  it("disables both arrow buttons when every card fits", () => {
    renderRow({ count: 3 });

    for (const button of screen.getAllByRole("button")) {
      expect(button.getAttribute("aria-disabled")).toBe("true");
    }
  });

  it("scrolls one page of whole cards smoothly", () => {
    stubMediaQueries(false);
    renderRow({ count: 20 });

    fireEvent.click(screen.getByRole("button", { name: "Scroll Recently added forward" }));

    const wholeCards = Math.floor(rowWidth / cardStride("poster"));
    expect(scrollTo).toHaveBeenCalledWith({
      left: wholeCards * cardStride("poster"),
      behavior: "smooth",
    });
  });

  it("jumps without animation when the user prefers reduced motion", () => {
    stubMediaQueries(true);
    renderRow({ count: 20 });
    scrollRowTo(cardStride("poster") * 10);

    fireEvent.click(screen.getByRole("button", { name: "Scroll Recently added back" }));

    expect(scrollTo).toHaveBeenCalledWith(expect.objectContaining({ behavior: "auto" }));
  });

  it("has no blocking accessibility violations", async () => {
    const { container } = renderRow({ count: 30 });

    expect(await blockingAxeViolations(container)).toEqual([]);
  });
});

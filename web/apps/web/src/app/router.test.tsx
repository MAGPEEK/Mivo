import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { renderApp } from "../test/renderApp";

function sidebar(): HTMLElement | null {
  return screen.queryByRole("navigation", { name: "Main navigation" });
}

describe("feature routes", () => {
  it.each([
    {
      path: "/libraries/lib-1",
      heading: "Library",
      text: "The videos of library lib-1 will be listed here.",
    },
    {
      path: "/items/item-1",
      heading: "Item details",
      text: "The details of item item-1 will be shown here.",
    },
  ])("renders $path inside the app layout", async ({ path, heading, text }) => {
    renderApp(path);

    const main = await screen.findByRole("main");

    expect(await within(main).findByRole("heading", { level: 1, name: heading })).toBeTruthy();
    expect(within(main).getByText(text)).toBeTruthy();
    expect(sidebar()).not.toBeNull();
    expect(screen.getByRole("banner")).toBeTruthy();
  });

  it("renders the player full screen, without sidebar and top bar", async () => {
    renderApp("/items/item-1/play");

    const main = await screen.findByRole("main");

    expect(await within(main).findByRole("heading", { level: 1, name: "Player" })).toBeTruthy();
    expect(sidebar()).toBeNull();
    expect(screen.queryByRole("banner")).toBeNull();
    expect(screen.queryByRole("link", { name: "Skip to content" })).toBeNull();
  });

  it("renders the feature pages in German from their own namespaces", async () => {
    renderApp("/libraries/lib-1", { languages: ["de"] });

    expect(await screen.findByRole("heading", { level: 1, name: "Bibliothek" })).toBeTruthy();
    expect(screen.getByText("Hier erscheinen die Videos der Bibliothek lib-1.")).toBeTruthy();
  });

  it("shows the not-found view in the app layout below a feature route", async () => {
    renderApp("/items/item-1/play/extra");

    expect(await screen.findByRole("heading", { level: 1, name: "Page not found" })).toBeTruthy();
    expect(sidebar()).not.toBeNull();
  });
});

describe("player route search", () => {
  it("passes a valid file and start position to the player", async () => {
    const { router } = renderApp("/items/item-1/play?fileId=file-1&t=90.5");

    expect(await screen.findByText("Item item-1, file file-1, from 90.5 s.")).toBeTruthy();
    expect(router.state.matches.at(-1)?.search).toEqual({ fileId: "file-1", t: 90.5 });
  });

  it("drops invalid values and still opens the player", async () => {
    const { router } = renderApp("/items/item-1/play?fileId=&t=-5&extra=1");

    expect(await screen.findByText("Item item-1, file primary file, from 0 s.")).toBeTruthy();
    expect(router.state.matches.at(-1)?.search).toMatchObject({ fileId: undefined, t: undefined });
  });

  it("builds player links that contain only the given values", () => {
    const { router } = renderApp("/");

    const location = router.buildLocation({
      to: "/items/$itemId/play",
      params: { itemId: "item-1" },
      search: { fileId: "file-1" },
    });

    expect(location.href).toBe("/items/item-1/play?fileId=file-1");
  });
});

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
      text: "This library does not exist.",
    },
    // "/items/$itemId" is covered by features/item/components/ItemPage.test.tsx ("in the app").
  ])("renders $path inside the app layout", async ({ path, heading, text }) => {
    renderApp(path);

    const main = await screen.findByRole("main");

    expect(await within(main).findByRole("heading", { level: 1, name: heading })).toBeTruthy();
    expect(await within(main).findByText(text)).toBeTruthy();
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
    expect(await screen.findByText("Diese Bibliothek gibt es nicht.")).toBeTruthy();
  });

  it("shows the not-found view in the app layout below a feature route", async () => {
    renderApp("/items/item-1/play/extra");

    expect(await screen.findByRole("heading", { level: 1, name: "Page not found" })).toBeTruthy();
    expect(sidebar()).not.toBeNull();
  });
});

describe("player route search", () => {
  // That the player plays what the search names: features/player/components/PlayerPage.test.tsx.
  it("validates a valid file and start position for the player", async () => {
    const { router } = renderApp("/items/item-1/play?fileId=file-1&t=90.5");

    expect(await screen.findByRole("heading", { level: 1, name: "Player" })).toBeTruthy();
    expect(router.state.matches.at(-1)?.search).toEqual({ fileId: "file-1", t: 90.5 });
  });

  it("drops invalid values and still opens the player", async () => {
    const { router } = renderApp("/items/item-1/play?fileId=&t=-5&extra=1");

    expect(await screen.findByRole("heading", { level: 1, name: "Player" })).toBeTruthy();
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

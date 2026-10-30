import { act, fireEvent, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { renderApp } from "../test/renderApp";

/** Waits for the routed page and returns the text of its single top-level heading. */
async function pageHeadingText(): Promise<string | null> {
  const heading = await screen.findByRole("heading", { level: 1 });
  return heading.textContent;
}

describe("App routing", () => {
  it("renders the home page at /", async () => {
    renderApp("/");

    expect(await pageHeadingText()).toBe("Home");
  });

  it("renders the not-found view for an unknown path", async () => {
    renderApp("/does-not-exist");

    expect(await pageHeadingText()).toBe("Page not found");
    expect(screen.queryByRole("heading", { name: "Home" })).toBeNull();
  });

  it("links from the not-found view back to home", async () => {
    const { router } = renderApp("/does-not-exist");
    const backHome = await screen.findByRole("link", { name: "Back to home" });

    expect(backHome.getAttribute("href")).toBe("/");
    fireEvent.click(backHome);

    await screen.findByRole("heading", { level: 1, name: "Home" });
    expect(router.state.location.pathname).toBe("/");
  });
});

describe("App language", () => {
  it("renders German when the browser prefers German", async () => {
    renderApp("/", { languages: ["de-DE", "en"] });

    expect(await pageHeadingText()).toBe("Startseite");
    expect(document.documentElement.lang).toBe("de");
  });

  it("re-renders and updates <html lang> when the language changes", async () => {
    const { i18n } = renderApp("/");
    expect(await pageHeadingText()).toBe("Home");
    expect(document.documentElement.lang).toBe("en");

    await act(() => i18n.changeLanguage("de"));

    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Startseite");
    expect(document.documentElement.lang).toBe("de");
  });
});

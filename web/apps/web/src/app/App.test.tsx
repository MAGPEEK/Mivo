import { QueryClient } from "@tanstack/react-query";
import { createMemoryHistory } from "@tanstack/react-router";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { createI18n } from "../shared/i18n";
import { App } from "./App";
import { createAppRouter } from "./router";

function renderApp(path: string, preferredLanguages: readonly string[] = ["en"]) {
  const i18n = createI18n(preferredLanguages);
  const router = createAppRouter({ history: createMemoryHistory({ initialEntries: [path] }) });
  render(<App i18n={i18n} queryClient={new QueryClient()} router={router} />);
  return { i18n, router };
}

/** Waits for the routed page and returns the text of its single top-level heading. */
async function pageHeadingText(): Promise<string | null> {
  const heading = await screen.findByRole("heading", { level: 1 });
  return heading.textContent;
}

describe("App routing", () => {
  it("renders the home placeholder at /", async () => {
    renderApp("/");

    expect(await pageHeadingText()).toBe("Home");
    expect(screen.getByRole("main").textContent).toContain("Your libraries will appear here.");
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
    renderApp("/", ["de-DE", "en"]);

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

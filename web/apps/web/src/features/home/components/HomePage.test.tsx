import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { renderApp } from "../../../test/renderApp";

describe("HomePage", () => {
  it("shows the empty library state", async () => {
    renderApp("/");

    const main = await screen.findByRole("main");

    expect(
      await within(main).findByRole("heading", { level: 2, name: "Your library is empty" }),
    ).toBeTruthy();
    expect(
      within(main).getByText(
        "Add folders under libraries in config.yaml, then restart the server. Your movies and shows appear here.",
      ),
    ).toBeTruthy();
  });

  it("shows the empty library state in German when the language is de", async () => {
    renderApp("/", { languages: ["de"] });

    const main = await screen.findByRole("main");

    expect(await within(main).findByRole("heading", { level: 1, name: "Startseite" })).toBeTruthy();
    expect(
      within(main).getByRole("heading", { level: 2, name: "Deine Bibliothek ist leer" }),
    ).toBeTruthy();
    expect(
      within(main).getByText(
        "Trage Ordner unter libraries in der config.yaml ein und starte den Server neu. Dann erscheinen hier deine Filme und Serien.",
      ),
    ).toBeTruthy();
  });
});

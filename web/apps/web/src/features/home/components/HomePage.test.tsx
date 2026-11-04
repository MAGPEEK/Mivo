import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { createFakeServer } from "../../../test/fakeApi";
import { fakeLibrary, librariesRoute } from "../../../test/libraryFixtures";
import { renderApp } from "../../../test/renderApp";

describe("HomePage", () => {
  it("leads to Settings › Libraries when no library exists", async () => {
    renderApp("/");

    const main = await screen.findByRole("main");

    expect(
      await within(main).findByRole("heading", { level: 2, name: "No libraries yet" }),
    ).toBeTruthy();
    const add = within(main).getByRole("link", { name: "Add library" });
    expect(add.getAttribute("href")).toContain("/admin/manage/libraries");
  });

  it("shows the empty state in German when the language is de", async () => {
    renderApp("/", { languages: ["de"] });

    const main = await screen.findByRole("main");

    expect(await within(main).findByRole("heading", { level: 1, name: "Startseite" })).toBeTruthy();
    expect(
      await within(main).findByRole("heading", { level: 2, name: "Noch keine Bibliotheken" }),
    ).toBeTruthy();
    expect(within(main).getByRole("link", { name: "Bibliothek hinzufügen" })).toBeTruthy();
  });

  it("points to the sidebar once libraries exist", async () => {
    renderApp("/", { fetch: createFakeServer([librariesRoute([fakeLibrary()])]).fetch });

    const main = await screen.findByRole("main");

    expect(
      await within(main).findByRole("heading", { level: 2, name: "Nothing to show yet" }),
    ).toBeTruthy();
    expect(within(main).queryByRole("link", { name: "Add library" })).toBeNull();
  });
});

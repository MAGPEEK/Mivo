import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import { createFakeServer, fakeSystemInfo, problemResponse } from "../../../test/fakeApi";
import { renderApp } from "../../../test/renderApp";

const aboutPath = "/admin/settings/about";
const serverErrorStatus = 500;

describe("AboutSettingsPage", () => {
  it("shows the server version and the provider credits", async () => {
    renderApp(aboutPath);
    const main = await screen.findByRole("main");

    expect(await within(main).findByText(fakeSystemInfo.version)).toBeTruthy();
    expect(within(main).getByRole("heading", { level: 1, name: "About" })).toBeTruthy();
    const credits = within(main).getByRole("region", { name: "Data sources" });
    expect(within(credits).getByRole("img", { name: "TMDB" })).toBeTruthy();
    expect(within(credits).getByText(/not endorsed or certified by TMDB/)).toBeTruthy();
    expect(within(credits).getByText(/Cover Art Archive/)).toBeTruthy();
    expect(within(main).getByText(/sends no telemetry/)).toBeTruthy();
    expect(await blockingAxeViolations(main)).toEqual([]);
  });

  it("says the version is unknown when the server info fails", async () => {
    const server = createFakeServer([
      {
        method: "GET",
        path: "/api/v1/system/info",
        respond: () => problemResponse(serverErrorStatus, "internal_error"),
      },
    ]);
    renderApp(aboutPath, { fetch: server.fetch });

    expect(await screen.findByText("Unknown", {}, { timeout: 3000 })).toBeTruthy();
  });

  it("speaks German but keeps the TMDB notice in its English wording", async () => {
    renderApp(aboutPath, { languages: ["de"] });

    expect(await screen.findByRole("heading", { level: 1, name: "Info" })).toBeTruthy();
    const notice = screen.getByText(
      "This product uses the TMDB API but is not endorsed or certified by TMDB.",
    );
    expect(notice.getAttribute("lang")).toBe("en");
    expect(
      screen.getByText(
        "Dieses Produkt nutzt die TMDB-API, wird aber von TMDB weder unterstützt noch zertifiziert.",
      ),
    ).toBeTruthy();
  });
});

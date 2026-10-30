import type { Problem } from "@mivo/api-client";
import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import {
  createFakeServer,
  fakeSystemInfo,
  jsonResponse,
  problemResponse,
  respondNever,
  respondWithProblem,
} from "../../../test/fakeApi";
import { renderApp } from "../../../test/renderApp";

const internalError: Problem = {
  type: "about:blank",
  title: "Internal Server Error",
  status: 500,
  code: "internal",
};

async function findVersionStatus(): Promise<HTMLElement> {
  const sidebar = await screen.findByRole("navigation", { name: "Main navigation" });
  return within(sidebar).getByRole("status");
}

const systemInfoPath = "/api/v1/system/info";

describe("ServerVersion in the sidebar footer", () => {
  it("shows an accessible skeleton while the version loads", async () => {
    renderApp("/", { fetch: respondNever() });

    const status = await findVersionStatus();

    expect(status.textContent).toBe("Loading server version");
  });

  it("renders the version from the fake response", async () => {
    const server = createFakeServer([
      {
        method: "GET",
        path: systemInfoPath,
        respond: () => jsonResponse({ ...fakeSystemInfo, version: "9.8.7" }),
      },
    ]);
    renderApp("/", { fetch: server.fetch });

    const status = await findVersionStatus();

    expect(await within(status).findByText("Server version 9.8.7")).toBeTruthy();
    expect(server.countRequests("GET", systemInfoPath)).toBe(1);
  });

  it("renders the error state on an ApiError after one retry", async () => {
    const server = createFakeServer([
      { method: "GET", path: systemInfoPath, respond: () => problemResponse(500, "internal") },
    ]);
    renderApp("/", { fetch: server.fetch });

    const status = await findVersionStatus();

    expect(await within(status).findByText("Server unreachable")).toBeTruthy();
    expect(server.countRequests("GET", systemInfoPath)).toBe(2);
  });

  it("renders the error state when the server cannot be reached", async () => {
    renderApp("/", { fetch: () => Promise.reject(new TypeError("Failed to fetch")) });

    const status = await findVersionStatus();

    expect(await within(status).findByText("Server unreachable")).toBeTruthy();
  });

  it("renders the error state in German when the language is de", async () => {
    renderApp("/", { languages: ["de"], fetch: respondWithProblem(internalError) });

    const sidebar = await screen.findByRole("navigation", { name: "Hauptnavigation" });

    expect(
      await within(within(sidebar).getByRole("status")).findByText("Server nicht erreichbar"),
    ).toBeTruthy();
  });
});

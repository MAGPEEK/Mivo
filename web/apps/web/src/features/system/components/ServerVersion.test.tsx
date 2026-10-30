import type { Problem } from "@mivo/api-client";
import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import {
  fakeSystemInfo,
  respondNever,
  respondWithJson,
  respondWithProblem,
  type FakeFetch,
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

// Counts the requests, so a test can check how often the client asked.
function counting(fetch: FakeFetch): { fetch: FakeFetch; requests: Request[] } {
  const requests: Request[] = [];
  return {
    requests,
    fetch: (request) => {
      requests.push(request);
      return fetch(request);
    },
  };
}

describe("ServerVersion in the sidebar footer", () => {
  it("shows an accessible skeleton while the version loads", async () => {
    renderApp("/", { fetch: respondNever() });

    const status = await findVersionStatus();

    expect(status.textContent).toBe("Loading server version");
  });

  it("renders the version from the fake response", async () => {
    const server = counting(respondWithJson(fakeSystemInfo, 200));
    renderApp("/", { fetch: server.fetch });

    const status = await findVersionStatus();

    expect(
      await within(status).findByText(`Server version ${fakeSystemInfo.version}`),
    ).toBeTruthy();
    expect(server.requests.map((request) => new URL(request.url).pathname)).toEqual([
      "/api/v1/system/info",
    ]);
  });

  it("renders the error state on an ApiError after one retry", async () => {
    const server = counting(respondWithProblem(internalError));
    renderApp("/", { fetch: server.fetch });

    const status = await findVersionStatus();

    expect(await within(status).findByText("Server unreachable")).toBeTruthy();
    expect(server.requests).toHaveLength(2);
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

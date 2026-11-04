import type { Library } from "@mivo/api-client";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import {
  createFakeServer,
  jsonResponse,
  problemResponse,
  type FakeRoute,
} from "../../../test/fakeApi";
import { fakeLibrary, finishedScan, rootUnavailableScan } from "../../../test/libraryFixtures";
import { renderApp } from "../../../test/renderApp";

const settingsPath = "/admin/manage/libraries";
const noContentStatus = 204;

const movies = fakeLibrary({ scan: finishedScan });
const archive = fakeLibrary({
  id: "9c8b7a6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d",
  name: "Archive",
  kind: "shows",
  itemCount: 1,
  importedFromConfig: true,
  folders: [
    { id: "f-1", path: "/mnt/archive/shows", available: true },
    { id: "f-2", path: "/mnt/offline/shows", available: false },
  ],
  scan: rootUnavailableScan,
});

/** A fake server with a mutable library list and a DELETE route that edits it. */
function serverWith(initial: Library[], deleteResponse?: () => Response) {
  const libraries = [...initial];
  const routes: FakeRoute[] = [
    {
      method: "GET",
      path: "/api/v1/libraries",
      respond: () => jsonResponse({ libraries }),
    },
    {
      method: "DELETE",
      path: "/api/v1/libraries/:libraryId",
      respond: ({ params }) => {
        if (deleteResponse !== undefined) {
          return deleteResponse();
        }
        const index = libraries.findIndex((library) => library.id === params["libraryId"]);
        libraries.splice(index, 1);
        return new Response(null, { status: noContentStatus });
      },
    },
  ];
  return createFakeServer(routes);
}

async function main(): Promise<HTMLElement> {
  return screen.findByRole("main");
}

describe("Settings › Libraries", () => {
  it("lists every library with folders, count, scan result and hints", async () => {
    renderApp(settingsPath, { fetch: serverWith([archive, movies]).fetch });

    const page = await main();
    const list = await within(page).findByRole("list", { name: "Libraries" });
    const rows = [...list.querySelectorAll<HTMLElement>(":scope > li")];

    expect(within(page).getByRole("heading", { level: 1, name: "Libraries" })).toBeTruthy();
    expect(within(page).getByRole("navigation", { name: "Breadcrumb" }).textContent).toContain(
      "Settings",
    );
    expect(rows).toHaveLength(2);
    const archiveRow = rows[0];
    const moviesRow = rows[1];
    expect(archiveRow).toBeDefined();
    expect(moviesRow).toBeDefined();
    if (archiveRow === undefined || moviesRow === undefined) {
      return;
    }
    expect(within(archiveRow).getByRole("heading", { name: "Archive" })).toBeTruthy();
    expect(within(archiveRow).getByText("/mnt/archive/shows")).toBeTruthy();
    expect(within(archiveRow).getByText("/mnt/offline/shows")).toBeTruthy();
    expect(within(archiveRow).getAllByText("Unavailable")).toHaveLength(1);
    expect(within(archiveRow).getByText("1 item")).toBeTruthy();
    expect(within(archiveRow).getByText("Imported from the configuration")).toBeTruthy();
    expect(within(archiveRow).getByText(/Scan failed/)).toBeTruthy();
    expect(within(moviesRow).getByText("2 items")).toBeTruthy();
    expect(within(moviesRow).getByText(/3 added/)).toBeTruthy();
    expect(within(moviesRow).queryByText("Imported from the configuration")).toBeNull();
    expect(moviesRow.querySelector("time")?.getAttribute("datetime")).toBe("2026-10-30T18:04:31Z");
  });

  it("shows the empty state with an enabled add button", async () => {
    renderApp(settingsPath, { fetch: serverWith([]).fetch });

    const page = await main();

    expect(
      await within(page).findByRole("heading", { level: 2, name: "No libraries yet" }),
    ).toBeTruthy();
    expect(within(page).getByRole("button", { name: "Add library" })).toHaveProperty(
      "disabled",
      false,
    );
  });

  it("shows a load error and retries", async () => {
    let failing = true;
    const server = createFakeServer([
      {
        method: "GET",
        path: "/api/v1/libraries",
        respond: () =>
          failing ? problemResponse(500, "internal") : jsonResponse({ libraries: [movies] }),
      },
    ]);
    renderApp(settingsPath, { fetch: server.fetch });

    const page = await main();
    expect(await within(page).findByText("The libraries could not be loaded.")).toBeTruthy();
    failing = false;
    fireEvent.click(within(page).getByRole("button", { name: "Try again" }));

    expect(await within(page).findByRole("heading", { name: "Movies" })).toBeTruthy();
  });

  it("scans a library with the keyboard", async () => {
    const server = createFakeServer([
      {
        method: "GET",
        path: "/api/v1/libraries",
        respond: () => jsonResponse({ libraries: [movies] }),
      },
      {
        method: "POST",
        path: `/api/v1/libraries/${movies.id}/scan`,
        respond: () => jsonResponse(finishedScan, 202),
      },
    ]);
    renderApp(settingsPath, { fetch: server.fetch });

    const scanButton = await screen.findByRole("button", { name: "Scan Movies" });
    scanButton.focus();
    expect(document.activeElement).toBe(scanButton);
    fireEvent.click(scanButton);

    await waitFor(() => {
      expect(server.countRequests("POST", `/api/v1/libraries/${movies.id}/scan`)).toBe(1);
    });
  });

  describe("removing", () => {
    it("asks first, names the library and keeps it on cancel", async () => {
      const server = serverWith([movies]);
      renderApp(settingsPath, { fetch: server.fetch });

      const opener = await screen.findByRole("button", { name: "Remove Movies" });
      opener.focus();
      fireEvent.click(opener);
      const dialog = screen.getByRole("dialog", { name: "Remove library" });

      expect(dialog.textContent).toContain("“Movies”");
      expect(dialog.textContent).toContain("Your media files are not touched.");
      expect(document.activeElement).toBe(within(dialog).getByRole("button", { name: "Cancel" }));
      fireEvent.keyDown(dialog, { key: "Escape" });

      expect(screen.queryByRole("dialog")).toBeNull();
      expect(server.countRequests("DELETE", `/api/v1/libraries/${movies.id}`)).toBe(0);
      expect(document.activeElement).toBe(screen.getByRole("button", { name: "Remove Movies" }));
    });

    it("keeps focus inside the dialog when tabbing", async () => {
      renderApp(settingsPath, { fetch: serverWith([movies]).fetch });

      fireEvent.click(await screen.findByRole("button", { name: "Remove Movies" }));
      const dialog = screen.getByRole("dialog");
      const cancel = within(dialog).getByRole("button", { name: "Cancel" });
      const confirm = within(dialog).getByRole("button", { name: "Remove library" });
      confirm.focus();
      fireEvent.keyDown(confirm, { key: "Tab" });
      expect(document.activeElement).toBe(cancel);
      fireEvent.keyDown(cancel, { key: "Tab", shiftKey: true });

      expect(document.activeElement).toBe(confirm);
    });

    it("deletes, refreshes the list and the sidebar, and focuses the heading", async () => {
      const server = serverWith([movies, archive]);
      renderApp(settingsPath, { fetch: server.fetch });
      const sidebar = await screen.findByRole("navigation", { name: "Main navigation" });
      expect(await within(sidebar).findByRole("link", { name: "Movies" })).toBeTruthy();

      fireEvent.click(await screen.findByRole("button", { name: "Remove Movies" }));
      fireEvent.click(screen.getByRole("button", { name: "Remove library" }));

      await waitFor(() => {
        expect(screen.queryByRole("heading", { level: 2, name: "Movies" })).toBeNull();
      });
      expect(screen.queryByRole("dialog")).toBeNull();
      expect(within(sidebar).queryByRole("link", { name: "Movies" })).toBeNull();
      expect(server.countRequests("DELETE", `/api/v1/libraries/${movies.id}`)).toBe(1);
      expect(document.activeElement).toBe(screen.getByRole("heading", { level: 1 }));
    });

    it("explains a 403 with the local-network message and keeps the dialog", async () => {
      renderApp(settingsPath, {
        fetch: serverWith([movies], () => problemResponse(403, "forbidden")).fetch,
      });

      fireEvent.click(await screen.findByRole("button", { name: "Remove Movies" }));
      fireEvent.click(screen.getByRole("button", { name: "Remove library" }));

      const alert = await within(screen.getByRole("dialog")).findByRole("alert");
      expect(alert.textContent).toBe(
        "Library management is only available from the local network until accounts exist.",
      );
    });

    it("ignores Cancel while the delete is running", async () => {
      let release: () => void = () => undefined;
      const pending = new Promise<void>((resolve) => {
        release = resolve;
      });
      const server = createFakeServer([
        {
          method: "GET",
          path: "/api/v1/libraries",
          respond: () => jsonResponse({ libraries: [movies] }),
        },
        {
          method: "DELETE",
          path: "/api/v1/libraries/:libraryId",
          respond: async () => {
            await pending;
            return problemResponse(403, "forbidden");
          },
        },
      ]);
      renderApp(settingsPath, { fetch: server.fetch });

      fireEvent.click(await screen.findByRole("button", { name: "Remove Movies" }));
      fireEvent.click(screen.getByRole("button", { name: "Remove library" }));
      await waitFor(() => {
        expect(server.countRequests("DELETE", `/api/v1/libraries/${movies.id}`)).toBe(1);
      });
      fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

      expect(screen.getByRole("dialog")).toBeTruthy();
      release();
      expect(await within(screen.getByRole("dialog")).findByText(/local network/)).toBeTruthy();
    });

    it("shows other failures with the problem detail", async () => {
      renderApp(settingsPath, {
        fetch: serverWith([movies], () =>
          jsonResponse(
            {
              type: "about:blank",
              title: "Unavailable",
              status: 503,
              code: "unavailable",
              detail: "Management is not ready yet.",
            },
            503,
            "application/problem+json",
          ),
        ).fetch,
      });

      fireEvent.click(await screen.findByRole("button", { name: "Remove Movies" }));
      fireEvent.click(screen.getByRole("button", { name: "Remove library" }));

      const alert = await within(screen.getByRole("dialog")).findByRole("alert");
      expect(alert.textContent).toBe(
        "The library could not be removed: Management is not ready yet.",
      );
    });
  });

  it("renders in German", async () => {
    renderApp(settingsPath, { fetch: serverWith([archive]).fetch, languages: ["de"] });

    const page = await main();

    expect(
      await within(page).findByRole("heading", { level: 1, name: "Bibliotheken" }),
    ).toBeTruthy();
    expect(within(page).getByText("Aus der Konfiguration übernommen")).toBeTruthy();
    fireEvent.click(within(page).getByRole("button", { name: "Archive entfernen" }));
    expect(screen.getByRole("dialog", { name: "Bibliothek entfernen" })).toBeTruthy();
  });

  it("has no serious accessibility violations, with the dialog open too", async () => {
    renderApp(settingsPath, { fetch: serverWith([archive, movies]).fetch });

    await screen.findByRole("button", { name: "Remove Movies" });
    expect(await blockingAxeViolations(document.body)).toEqual([]);
    fireEvent.click(screen.getByRole("button", { name: "Remove Movies" }));

    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });
});

describe("Empty library states", () => {
  it("leads from Home to the add dialog route", async () => {
    const { router } = renderApp("/", { fetch: serverWith([]).fetch });

    const main_ = await main();
    fireEvent.click(await within(main_).findByRole("link", { name: "Add library" }));

    await waitFor(() => {
      expect(router.state.location.pathname).toBe(settingsPath);
    });
    expect(router.state.location.search).toEqual({ add: true });
  });

  it("offers the same link in the sidebar and the settings entry", async () => {
    const { router } = renderApp("/", { fetch: serverWith([]).fetch });

    const sidebar = await screen.findByRole("navigation", { name: "Main navigation" });
    fireEvent.click(await within(sidebar).findByRole("link", { name: "Add library" }));
    await waitFor(() => {
      expect(router.state.location.pathname).toBe(settingsPath);
    });
    fireEvent.click(within(sidebar).getByRole("link", { name: "Settings" }));

    expect(router.state.location.pathname).toBe(settingsPath);
  });
});

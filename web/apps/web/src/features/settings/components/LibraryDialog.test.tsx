import type {
  FolderEntry,
  FolderListing,
  InstallKind,
  Library,
  LibraryKind,
} from "@mivo/api-client";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import {
  createFakeServer,
  fakeSystemInfo,
  jsonResponse,
  problemResponse,
  type FakeRoute,
} from "../../../test/fakeApi";
import { fakeLibrary } from "../../../test/libraryFixtures";
import { renderApp } from "../../../test/renderApp";

const settingsPath = "/admin/manage/libraries";
const addPath = `${settingsPath}?add=1`;
const created = 201;
const badRequest = 400;

const rootId = "0c1d2e3f-4a5b-4c6d-8e7f-9a0b1c2d3e4f";
const offlineRootId = "1d2e3f4a-5b6c-4d7e-8f90-a1b2c3d4e5f6";

const folderEntry = (name: string, path = name, extra: Partial<FolderEntry> = {}): FolderEntry => ({
  name,
  path,
  selectable: true,
  ...extra,
});

function listing(path: string, folders: FolderEntry[], extra: Partial<FolderListing> = {}) {
  return { rootId, path, folders, unreadableCount: 0, truncated: false, ...extra };
}

const rootListings: Record<string, FolderListing> = {
  "": listing("", [
    folderEntry("Movies"),
    folderEntry("Shows"),
    folderEntry("Used", "Used", { selectable: false, reason: "in_use" }),
    folderEntry("data", "data", { selectable: false, reason: "reserved" }),
  ]),
  Movies: listing("Movies", [folderEntry("Kids", "Movies/Kids")], { parentPath: "" }),
  Shows: listing("Shows", [], { parentPath: "" }),
};

interface ServerOptions {
  readonly libraries?: Library[];
  readonly installKind?: InstallKind;
  readonly listings?: Record<string, FolderListing>;
  readonly kinds?: LibraryKind[];
  /** Answers the save request; the default echoes a created or changed library. */
  readonly save?: () => Response;
}

/** Fake server for the dialog: roots, listings, libraries and the save requests. */
function dialogServer(options: ServerOptions = {}) {
  const { libraries = [], installKind = "standard", listings = rootListings } = options;
  const saved: { method: string; body: unknown }[] = [];
  const record = async (request: Request, status: number) => {
    saved.push({ method: request.method, body: await request.json() });
    return options.save?.() ?? jsonResponse(fakeLibrary({ name: "Saved" }), status);
  };
  const routes: FakeRoute[] = [
    {
      method: "GET",
      path: "/api/v1/system/info",
      respond: () =>
        jsonResponse({ ...fakeSystemInfo, libraryKinds: options.kinds ?? ["movies", "shows"] }),
    },
    { method: "GET", path: "/api/v1/libraries", respond: () => jsonResponse({ libraries }) },
    {
      method: "GET",
      path: "/api/v1/folder-roots",
      respond: () =>
        jsonResponse({
          roots: [
            { id: rootId, path: "/media", available: true },
            { id: offlineRootId, path: "/mnt/offline", available: false },
          ],
          installKind,
          serviceUser: "mivo",
        }),
    },
    {
      method: "GET",
      path: "/api/v1/folder-roots/:rootId/folders",
      respond: ({ url }) => {
        const found = listings[url.searchParams.get("path") ?? ""];
        return found === undefined ? problemResponse(404, "not_found") : jsonResponse(found);
      },
    },
    {
      method: "POST",
      path: "/api/v1/libraries",
      respond: ({ request }) => record(request, created),
    },
    {
      method: "PATCH",
      path: "/api/v1/libraries/:libraryId",
      respond: ({ request }) => record(request, 200),
    },
  ];
  return { server: createFakeServer(routes), saved };
}

function problem(code: string, field?: string, libraryId?: string): Response {
  return jsonResponse(
    { type: "about:blank", title: code, status: badRequest, code, field, libraryId },
    badRequest,
    "application/problem+json",
  );
}

const twoFolders = fakeLibrary({
  folders: [
    { id: "f-1", path: "/media/a", available: true },
    { id: "f-2", path: "/media/b", available: true },
  ],
});

async function openTab(name: string): Promise<void> {
  fireEvent.click(await screen.findByRole("tab", { name }));
}

/** Opens the folder browser, walks into `path` segments and chooses that folder. */
async function chooseFolder(...segments: string[]): Promise<void> {
  await openTab("Folders");
  fireEvent.click(screen.getByRole("button", { name: "Add folder" }));
  const browser = await screen.findByRole("dialog", { name: "Choose folder" });
  for (const segment of segments) {
    fireEvent.click(await within(browser).findByRole("button", { name: segment }));
  }
  // The chosen path is the browser's state, so the last listing need not have arrived.
  fireEvent.click(within(browser).getByRole("button", { name: "Select this folder" }));
}

function save(): void {
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
}

describe("Add library dialog", () => {
  it("adds a library from two folders, prefilling the name until it is edited", async () => {
    const { server, saved } = dialogServer();
    renderApp(addPath, { fetch: server.fetch });

    await chooseFolder("Movies");
    await chooseFolder("Shows");
    const folders = within(screen.getByRole("list", { name: "Folders" }));
    expect(folders.getByText("/media/Movies")).toBeTruthy();
    expect(folders.getByText("/media/Shows")).toBeTruthy();
    await openTab("General");
    const name = screen.getByRole("textbox", { name: "Name" });
    expect(name).toHaveProperty("value", "Movies");

    fireEvent.change(name, { target: { value: "My films" } });
    await openTab("Folders");
    fireEvent.click(screen.getByRole("button", { name: "Remove folder /media/Movies" }));
    await openTab("General");
    expect(screen.getByRole("textbox", { name: "Name" })).toHaveProperty("value", "My films");
    fireEvent.change(screen.getByRole("combobox", { name: "Metadata language" }), {
      target: { value: "de" },
    });
    save();

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).toBeNull();
    });
    expect(saved).toEqual([
      {
        method: "POST",
        body: {
          kind: "movies",
          name: "My films",
          folders: [{ rootId, path: "Shows" }],
          metadataLanguage: "de",
          options: { scanExtras: true, readNfo: true, localArtwork: true, onlineMetadata: true },
        },
      },
    ]);
    expect((await screen.findByText("Scanning Saved…")).closest("[role=status]")).toBeTruthy();
    expect(server.countRequests("GET", "/api/v1/libraries")).toBeGreaterThan(1);
  });

  it("offers only the kinds the server supports", async () => {
    const { server } = dialogServer({ kinds: ["shows"] });
    renderApp(addPath, { fetch: server.fetch });

    const group = await screen.findByRole("group", { name: "Kind of library" });

    expect(within(group).getAllByRole("radio")).toHaveLength(1);
    expect(within(group).getByRole("radio", { name: "Shows" })).toHaveProperty("checked", true);
  });

  it("offers a named music tile with its icon when the server supports music", async () => {
    const { server } = dialogServer({ kinds: ["movies", "shows", "music"] });
    renderApp(addPath, { fetch: server.fetch });

    const group = await screen.findByRole("group", { name: "Kind of library" });
    const music = within(group).getByRole("radio", { name: "Music" });

    expect(music.closest("label")?.querySelector('[data-icon="music"]')).not.toBeNull();
  });

  it("creates a shows library when the tile is chosen", async () => {
    const { server, saved } = dialogServer();
    renderApp(addPath, { fetch: server.fetch });

    fireEvent.click(await screen.findByRole("radio", { name: "Shows" }));
    await chooseFolder("Shows");
    save();

    await waitFor(() => {
      expect(saved).toHaveLength(1);
    });
    expect(saved[0]?.body).toMatchObject({ kind: "shows", name: "Shows" });
  });

  it("checks name and folders locally before asking the server", async () => {
    const { server, saved } = dialogServer();
    renderApp(addPath, { fetch: server.fetch });

    await screen.findByRole("textbox", { name: "Name" });
    save();
    expect(await screen.findByText("Enter a name for the library.")).toBeTruthy();
    fireEvent.change(screen.getByRole("textbox", { name: "Name" }), { target: { value: "X" } });
    save();

    expect(within(screen.getByRole("tabpanel")).getByText("Add at least one folder.")).toBeTruthy();
    expect(saved).toEqual([]);
  });

  it("keeps the expert options behind the switch and sends them", async () => {
    const { server, saved } = dialogServer();
    renderApp(addPath, { fetch: server.fetch });

    await screen.findByRole("textbox", { name: "Name" });
    expect(screen.queryByRole("tab", { name: "Advanced" })).toBeNull();
    fireEvent.click(screen.getByRole("switch", { name: "Expert settings" }));
    await openTab("Advanced");
    const nfo = screen.getByRole("checkbox", { name: "Read NFO files" });
    expect(screen.getByRole("checkbox", { name: "Use local artwork" })).toHaveProperty(
      "checked",
      true,
    );
    fireEvent.click(nfo);
    fireEvent.click(screen.getByRole("checkbox", { name: "Use online metadata" }));
    await chooseFolder("Movies");
    save();

    await waitFor(() => {
      expect(saved).toHaveLength(1);
    });
    expect(saved[0]?.body).toMatchObject({
      options: { scanExtras: true, readNfo: false, localArtwork: true, onlineMetadata: false },
    });
  });

  it("leaves the Advanced tab when the switch goes off", async () => {
    renderApp(addPath, { fetch: dialogServer().server.fetch });

    await screen.findByRole("textbox", { name: "Name" });
    fireEvent.click(screen.getByRole("switch", { name: "Expert settings" }));
    await openTab("Advanced");
    fireEvent.click(screen.getByRole("switch", { name: "Expert settings" }));

    expect(screen.queryByRole("tab", { name: "Advanced" })).toBeNull();
    expect(screen.getByRole("textbox", { name: "Name" })).toBeTruthy();
  });
});

describe("Edit library dialog", () => {
  const editPath = `${settingsPath}?edit=${twoFolders.id}`;

  it("keeps folder ids, locks the kind and omits it from the request", async () => {
    const { server, saved } = dialogServer({ libraries: [twoFolders] });
    renderApp(editPath, { fetch: server.fetch });

    const dialog = await screen.findByRole("dialog", { name: "Edit library" });
    expect(within(dialog).getAllByRole("radio")).toHaveLength(1);
    expect(within(dialog).getByRole("radio", { name: "Movies" })).toHaveProperty("disabled", true);
    expect(within(dialog).getByText(/Create a new library to change the kind/)).toBeTruthy();
    expect(within(dialog).getByRole("textbox", { name: "Name" })).toHaveProperty("value", "Movies");
    await openTab("Folders");
    fireEvent.click(screen.getByRole("button", { name: "Remove folder /media/a" }));
    await chooseFolder("Shows");
    save();

    await waitFor(() => {
      expect(saved).toHaveLength(1);
    });
    expect(saved[0]).toEqual({
      method: "PATCH",
      body: {
        name: "Movies",
        folders: [{ folderId: "f-2" }, { rootId, path: "Shows" }],
        metadataLanguage: "",
        options: { scanExtras: true, readNfo: true, localArtwork: true },
      },
    });
  });

  it("opens from the Edit button and writes the state to the URL", async () => {
    renderApp(settingsPath, { fetch: dialogServer({ libraries: [twoFolders] }).server.fetch });

    fireEvent.click(await screen.findByRole("button", { name: "Edit Movies" }));

    expect(await screen.findByRole("dialog", { name: "Edit library" })).toBeTruthy();
  });

  it("asks before discarding changes and closes without them", async () => {
    const { router } = renderApp(editPath, {
      fetch: dialogServer({ libraries: [twoFolders] }).server.fetch,
    });

    const name = await screen.findByRole("textbox", { name: "Name" });
    fireEvent.change(name, { target: { value: "Renamed" } });
    fireEvent.keyDown(screen.getByRole("dialog", { name: "Edit library" }), { key: "Escape" });
    const confirm = screen.getByRole("dialog", { name: "Discard changes?" });
    fireEvent.click(within(confirm).getByRole("button", { name: "Keep editing" }));

    expect(screen.queryByRole("dialog", { name: "Discard changes?" })).toBeNull();
    expect(screen.getByRole("textbox", { name: "Name" })).toHaveProperty("value", "Renamed");
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    fireEvent.click(screen.getByRole("button", { name: "Discard" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).toBeNull();
    });
    expect(router.state.location.search).toEqual({});
  });

  it("closes an unchanged dialog at once with Escape and returns focus", async () => {
    renderApp(settingsPath, { fetch: dialogServer({ libraries: [twoFolders] }).server.fetch });
    const opener = await screen.findByRole("button", { name: "Edit Movies" });
    opener.focus();
    fireEvent.click(opener);

    const dialog = await screen.findByRole("dialog", { name: "Edit library" });
    expect(document.activeElement).toBe(within(dialog).getByRole("textbox", { name: "Name" }));
    fireEvent.keyDown(dialog, { key: "Escape" });

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).toBeNull();
    });
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Edit Movies" }));
  });

  it("moves between tabs with the arrow keys and wraps Tab inside the dialog", async () => {
    renderApp(editPath, { fetch: dialogServer({ libraries: [twoFolders] }).server.fetch });

    const dialog = await screen.findByRole("dialog", { name: "Edit library" });
    const general = within(dialog).getByRole("tab", { name: "General" });
    general.focus();
    fireEvent.keyDown(general, { key: "ArrowRight" });
    expect(within(dialog).getByRole("tab", { name: "Folders", selected: true })).toBeTruthy();
    expect(document.activeElement).toBe(within(dialog).getByRole("tab", { name: "Folders" }));
    const save_ = within(dialog).getByRole("button", { name: "Save" });
    save_.focus();
    fireEvent.keyDown(save_, { key: "Tab" });

    expect(document.activeElement).toBe(within(dialog).getByRole("button", { name: "Close" }));
  });
});

describe("Save problems", () => {
  const editPath = `${settingsPath}?edit=${twoFolders.id}`;
  const other = fakeLibrary({ id: "lib-other", name: "Archive", folders: [] });

  async function saveWith(response: () => Response) {
    renderApp(editPath, {
      fetch: dialogServer({ libraries: [twoFolders, other], save: response }).server.fetch,
    });
    await screen.findByRole("dialog", { name: "Edit library" });
    save();
  }

  it.each([
    ["name_invalid", "name", "The name must have 1 to 128 characters."],
    ["conflict", "name", "Another library already has this name."],
  ])("shows %s at the name field", async (code, field, text) => {
    await saveWith(() => problem(code, field));

    const name = await screen.findByRole("textbox", { name: "Name" });
    expect(name.getAttribute("aria-invalid")).toBe("true");
    expect(within(screen.getByRole("tabpanel")).getByText(text)).toBeTruthy();
  });

  it("shows language_invalid at the language field", async () => {
    await saveWith(() => problem("language_invalid", "metadataLanguage"));

    expect(await screen.findByText("The server does not know this language.")).toBeTruthy();
    expect(screen.getByRole("combobox", { name: "Metadata language" })).toBeTruthy();
  });

  it("shows kind_unsupported below the kind tiles", async () => {
    await saveWith(() => problem("kind_unsupported", "kind"));

    const group = await screen.findByRole("group", { name: "Kind of library" });
    expect(
      within(group).getByText("This server does not support this kind of library."),
    ).toBeTruthy();
  });

  it.each([
    ["folder_not_found", "This folder does not exist (any more)."],
    ["folder_unreadable", "The server cannot read this folder."],
    ["folder_overlap", "This folder overlaps a folder of another library."],
    ["folder_reserved", "The server uses this folder itself, so it cannot be a library."],
    ["folder_outside_roots", "This folder is outside the folders the server allows."],
  ])("shows %s at the second folder", async (code, text) => {
    await saveWith(() => problem(code, "folders[1]"));

    const items = within(await screen.findByRole("tabpanel")).getAllByRole("listitem");
    expect(items[1]?.textContent).toContain("/media/b");
    expect(items[1]?.textContent).toContain(text);
    expect(items[0]?.textContent).not.toContain(text);
  });

  it("names the other library for folder_overlap when it is known", async () => {
    await saveWith(() => problem("folder_overlap", "folders[0]", "lib-other"));

    expect(await screen.findByText("This folder overlaps a folder of “Archive”.")).toBeTruthy();
  });

  it("shows too_many_folders at the folder list", async () => {
    await saveWith(() => problem("too_many_folders", "folders"));

    expect(await screen.findByText("A library can have at most 16 folders.")).toBeTruthy();
    expect(screen.getByRole("tab", { name: "Folders", selected: true })).toBeTruthy();
  });

  it("shows unknown codes with the server reason and the 403 as local-network text", async () => {
    await saveWith(() =>
      jsonResponse(
        {
          type: "about:blank",
          title: "Boom",
          status: 500,
          code: "internal_error",
          detail: "Disk gone",
        },
        500,
        "application/problem+json",
      ),
    );
    expect(await screen.findByText("The library could not be saved: Disk gone")).toBeTruthy();
  });

  it("explains a 403", async () => {
    await saveWith(() => problemResponse(403, "forbidden"));

    expect(await screen.findByText(/only available from the local network/)).toBeTruthy();
  });

  it("clears the message once the input changes", async () => {
    await saveWith(() => problem("name_invalid", "name"));
    await screen.findByText("The name must have 1 to 128 characters.");

    fireEvent.change(screen.getByRole("textbox", { name: "Name" }), { target: { value: "Ok" } });

    expect(screen.queryByText("The name must have 1 to 128 characters.")).toBeNull();
  });
});

describe("Permission help", () => {
  const helpName = "How to give the server access";

  it.each([
    ["synology", ["Control Panel › Shared Folder", "System internal user", "Read only", "mivo"]],
    ["docker", ["below /media", "-v /volume1/video:/media/video:ro", "restart the container"]],
    ["standard", ["Give the user mivo read access to the folder."]],
  ] as const)("explains %s installs after folder_unreadable", async (installKind, fragments) => {
    const { server } = dialogServer({
      libraries: [twoFolders],
      installKind,
      save: () => problem("folder_unreadable", "folders[0]"),
    });
    renderApp(`${settingsPath}?edit=${twoFolders.id}`, { fetch: server.fetch });
    await screen.findByRole("dialog", { name: "Edit library" });
    save();

    const help = await screen.findByRole("region", { name: helpName });
    for (const fragment of fragments) {
      expect(help.textContent).toContain(fragment);
    }
  });

  it("shows no help for other folder problems", async () => {
    const { server } = dialogServer({
      libraries: [twoFolders],
      save: () => problem("folder_reserved", "folders[0]"),
    });
    renderApp(`${settingsPath}?edit=${twoFolders.id}`, { fetch: server.fetch });
    await screen.findByRole("dialog", { name: "Edit library" });
    save();

    await screen.findByText(/uses this folder itself/);
    expect(screen.queryByRole("region", { name: helpName })).toBeNull();
  });

  it("explains unreadable folders in the browser, per install kind", async () => {
    const { server } = dialogServer({
      installKind: "docker",
      listings: { "": listing("", [folderEntry("Movies")], { unreadableCount: 3 }) },
    });
    renderApp(addPath, { fetch: server.fetch });
    await openTab("Folders");
    fireEvent.click(screen.getByRole("button", { name: "Add folder" }));

    const browser = await screen.findByRole("dialog", { name: "Choose folder" });

    expect(
      await within(browser).findByText("3 folders here cannot be read by the server."),
    ).toBeTruthy();
    expect(within(browser).getByRole("region", { name: helpName }).textContent).toContain(
      "restart the container",
    );
  });
});

describe("Folder browser", () => {
  async function openBrowser() {
    renderApp(addPath, { fetch: dialogServer().server.fetch });
    await openTab("Folders");
    fireEvent.click(screen.getByRole("button", { name: "Add folder" }));
    return screen.findByRole("dialog", { name: "Choose folder" });
  }

  it("marks unavailable roots and shows the breadcrumb and Up", async () => {
    const browser = await openBrowser();

    const offline = await within(browser).findByText("/mnt/offline (unavailable)");
    expect(offline).toHaveProperty("disabled", true);
    expect(within(browser).getByRole("button", { name: "Up" })).toHaveProperty("disabled", true);
    fireEvent.click(await within(browser).findByRole("button", { name: "Movies" }));
    await within(browser).findByRole("button", { name: "Kids" });

    const crumbs = within(within(browser).getByRole("navigation", { name: "Breadcrumb" }));
    expect(crumbs.getByRole("button", { name: "/media" })).toBeTruthy();
    expect(crumbs.getByText("Movies").getAttribute("aria-current")).toBe("page");
    fireEvent.click(within(browser).getByRole("button", { name: "Up" }));
    expect(await within(browser).findByRole("button", { name: "Shows" })).toBeTruthy();
  });

  it("shows the reason of folders that cannot be chosen and does not open them", async () => {
    const browser = await openBrowser();

    const used = await within(browser).findByRole("button", { name: /^Used/ });
    const reserved = within(browser).getByRole("button", { name: /^data/ });
    expect(used.getAttribute("aria-disabled")).toBe("true");
    expect(used.textContent).toContain("Already used by a library");
    expect(reserved.textContent).toContain("Used by the server");
    fireEvent.click(used);

    expect(within(browser).getByRole("button", { name: "Up" })).toHaveProperty("disabled", true);
    expect(within(browser).getByRole("button", { name: "Movies" })).toBeTruthy();
  });

  it("tells when the list is cut at 1000 folders", async () => {
    renderApp(addPath, {
      fetch: dialogServer({
        listings: { "": listing("", [folderEntry("Movies")], { truncated: true }) },
      }).server.fetch,
    });
    await openTab("Folders");
    fireEvent.click(screen.getByRole("button", { name: "Add folder" }));

    expect(await screen.findByText("Only the first 1000 folders are shown.")).toBeTruthy();
  });

  it("closes alone on Escape and returns focus to Add folder", async () => {
    renderApp(addPath, { fetch: dialogServer().server.fetch });
    await openTab("Folders");
    const opener = screen.getByRole("button", { name: "Add folder" });
    opener.focus();
    fireEvent.click(opener);
    const browser = await screen.findByRole("dialog", { name: "Choose folder" });

    fireEvent.keyDown(browser, { key: "Escape" });

    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "Choose folder" })).toBeNull();
    });
    expect(screen.getByRole("dialog", { name: "Add library" })).toBeTruthy();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Add folder" }));
  });

  it("selects the root itself", async () => {
    const browser = await openBrowser();
    await within(browser).findByRole("list", { name: "Sub-folders" });

    fireEvent.click(within(browser).getByRole("button", { name: "Select this folder" }));

    const folders = await screen.findByRole("list", { name: "Folders" });
    expect(within(folders).getByText("/media")).toBeTruthy();
  });

  it("scrolls only the folder list so Select this folder stays reachable (FIX-folder-select)", async () => {
    const manyFolders = Array.from({ length: 120 }, (_, index) =>
      folderEntry(`Movie folder ${String(index + 1).padStart(3, "0")}`),
    );
    renderApp(addPath, {
      fetch: dialogServer({ listings: { "": listing("", manyFolders) } }).server.fetch,
    });
    await openTab("Folders");
    fireEvent.click(screen.getByRole("button", { name: "Add folder" }));
    const browser = await screen.findByRole("dialog", { name: "Choose folder" });
    const list = await within(browser).findByRole("list", { name: "Sub-folders" });

    // jsdom has no layout, so the scroll region is found by its overflow class.
    const scrollRegion = list.closest(".overflow-y-auto");
    expect(scrollRegion).not.toBeNull();
    expect(
      scrollRegion?.contains(within(browser).getByRole("button", { name: "Movie folder 120" })),
    ).toBe(true);
    const fixed = [
      within(browser).getByRole("button", { name: "Select this folder" }),
      within(browser).getByRole("button", { name: "Cancel" }),
      within(browser).getByRole("button", { name: "Up" }),
      within(browser).getByRole("combobox", { name: "Location" }),
      within(browser).getByRole("navigation", { name: "Breadcrumb" }),
    ];
    for (const element of fixed) {
      expect(scrollRegion?.contains(element)).toBe(false);
    }
  });

  it("explains a failed listing", async () => {
    renderApp(addPath, {
      fetch: dialogServer({ listings: {} }).server.fetch,
    });
    await openTab("Folders");
    fireEvent.click(screen.getByRole("button", { name: "Add folder" }));

    expect(await screen.findByText(/The folders could not be loaded/)).toBeTruthy();
  });
});

describe("Library dialog in German and accessibility", () => {
  it("renders in German", async () => {
    renderApp(addPath, { fetch: dialogServer().server.fetch, languages: ["de"] });

    expect(await screen.findByRole("dialog", { name: "Bibliothek hinzufügen" })).toBeTruthy();
    expect(await screen.findByRole("textbox", { name: "Name" })).toBeTruthy();
    expect(screen.getByRole("combobox", { name: "Sprache der Metadaten" })).toBeTruthy();
    expect(screen.getByRole("option", { name: "Serverstandard" })).toBeTruthy();
    expect(screen.getByRole("option", { name: "Englisch" })).toBeTruthy();
  });

  it("has no serious violations on every tab and in the folder browser", async () => {
    const { server } = dialogServer({ libraries: [twoFolders] });
    renderApp(`${settingsPath}?edit=${twoFolders.id}`, { fetch: server.fetch });
    await screen.findByRole("dialog", { name: "Edit library" });
    expect(await blockingAxeViolations(document.body)).toEqual([]);

    fireEvent.click(screen.getByRole("switch", { name: "Expert settings" }));
    await openTab("Advanced");
    expect(await blockingAxeViolations(document.body)).toEqual([]);
    await openTab("Folders");
    expect(await blockingAxeViolations(document.body)).toEqual([]);
    fireEvent.click(screen.getByRole("button", { name: "Add folder" }));
    await screen.findByRole("button", { name: "Movies" });

    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });
});

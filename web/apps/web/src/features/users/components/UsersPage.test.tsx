import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import { jsonResponse, problemResponse } from "../../../test/fakeApi";
import { renderApp } from "../../../test/renderApp";
import {
  createFakeUsersServer,
  jonas,
  movies,
  shows,
  userMe,
  type FakeUsersServer,
} from "../test/fakeUsersServer";

const usersPath = "/admin/users";

async function table(): Promise<HTMLElement> {
  return screen.findByRole("table", { name: "Accounts and profiles" });
}

function rowOf(name: string): HTMLElement {
  const header = screen.getByRole("rowheader", { name: new RegExp(`^${name}`) });
  const row = header.closest("tr");
  if (row === null) {
    throw new Error(`no row for ${name}`);
  }
  return row;
}

function dialog(): HTMLElement {
  return screen.getByRole("dialog");
}

function lastBody(server: FakeUsersServer, method: string): unknown {
  return server.bodies.filter((entry) => entry.method === method).at(-1)?.body;
}

async function openEditor(name: string): Promise<HTMLElement> {
  await table();
  fireEvent.click(screen.getByRole("button", { name: `Edit ${name}` }));
  return dialog();
}

describe("Admin › Users", () => {
  it("lists accounts with their managed profiles nested below", async () => {
    const server = createFakeUsersServer();
    const { router } = renderApp(usersPath, { fetch: server.fetch });

    await table();
    const anna = rowOf("Anna");
    expect(within(anna).getByText("Admin")).toBeTruthy();
    expect(within(anna).getByText("Adults (no limit)")).toBeTruthy();
    expect(within(anna).getByText("All")).toBeTruthy();
    expect(within(anna).getByText("Active")).toBeTruthy();
    const jonasRow = rowOf("Jonas");
    expect(
      within(jonasRow).getByText("Custom (rated up to 16), hides unrated titles"),
    ).toBeTruthy();
    expect(within(jonasRow).getByText("1 library")).toBeTruthy();
    expect(within(jonasRow).getByText("Disabled")).toBeTruthy();
    expect(within(jonasRow).getByText("Never")).toBeTruthy();
    const maxRow = (await screen.findByText("Max")).closest("th");
    if (maxRow === null) {
      throw new Error("Max has no row header");
    }
    expect(maxRow.textContent).toContain("Managed profile of Anna");
    expect(within(maxRow).getByText("PIN")).toBeTruthy();
    const maxCells = maxRow.closest("tr")?.textContent ?? "";
    expect(maxCells).toContain("Children (rated up to 6), hides unrated titles");
    expect(maxCells).toContain("2 libraries");
    expect(screen.getByRole("link", { name: "Users" }).getAttribute("aria-current")).toBe("page");
    expect(router.state.location.pathname).toBe(usersPath);
    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });

  it("shows a clear state while the server does not manage accounts yet", async () => {
    const server = createFakeUsersServer({ list: () => problemResponse(501, "not_implemented") });
    renderApp(usersPath, { fetch: server.fetch });

    expect(
      await screen.findByRole("heading", { name: "User management is not available yet" }),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Add user" })).toBeNull();
  });

  it("shows the 501 state on a server without accounts and checks the session once", async () => {
    const notImplemented = () => problemResponse(501, "not_implemented");
    const server = createFakeUsersServer({ list: notImplemented }, [
      { method: "GET", path: "/api/v1/setup", respond: notImplemented },
      { method: "GET", path: "/api/v1/me", respond: notImplemented },
    ]);
    renderApp(usersPath, { fetch: server.fetch });

    expect(
      await screen.findByRole("heading", { name: "User management is not available yet" }),
    ).toBeTruthy();
    // A page that reads the session must not restart the failed check (it looped in e2e).
    await new Promise((resolve) => setTimeout(resolve, 100));
    expect(server.countRequests("GET", "/api/v1/setup")).toBe(1);
  });

  it("explains a refusal and offers a retry after other failures", async () => {
    renderApp(usersPath, {
      fetch: createFakeUsersServer({ list: () => problemResponse(403, "forbidden") }).fetch,
    });
    expect((await screen.findByRole("alert")).textContent).toContain(
      "Only an administrator on the local network",
    );
  });

  it("retries a failed list", async () => {
    let fail = true;
    const server = createFakeUsersServer({
      list: () =>
        fail ? problemResponse(500, "internal_error") : jsonResponse({ users: [jonas] }),
    });
    renderApp(usersPath, { fetch: server.fetch });

    const retry = await screen.findByRole("button", { name: "Try again" });
    fail = false;
    fireEvent.click(retry);
    expect(await table()).toBeTruthy();
  });

  it("creates a user with selected libraries and a custom limit", async () => {
    const server = createFakeUsersServer();
    renderApp(usersPath, { fetch: server.fetch });
    await table();
    fireEvent.click(screen.getByRole("button", { name: "Add user" }));
    const form = dialog();

    fireEvent.click(within(form).getByRole("button", { name: "Save" }));
    expect(within(form).getByText("Enter a name.")).toBeTruthy();
    expect(within(form).getByText("Enter an initial password.")).toBeTruthy();
    expect(server.bodies).toHaveLength(0);

    fireEvent.change(within(form).getByLabelText("Name"), { target: { value: "Ben" } });
    fireEvent.change(within(form).getByLabelText("Initial password"), {
      target: { value: "short" },
    });
    fireEvent.click(within(form).getByRole("button", { name: "Save" }));
    expect(within(form).getByText("Use at least 8 characters.")).toBeTruthy();

    fireEvent.change(within(form).getByLabelText("Initial password"), {
      target: { value: "nordlicht-2026" },
    });
    fireEvent.click(within(form).getByRole("radio", { name: "Selected libraries" }));
    fireEvent.click(await within(form).findByRole("checkbox", { name: "Shows" }));
    fireEvent.change(within(form).getByLabelText("Preset"), { target: { value: "custom" } });
    fireEvent.change(within(form).getByLabelText("Highest age rating"), {
      target: { value: "18" },
    });
    expect(within(form).getByRole("checkbox", { name: "Hide unrated titles" })).toHaveProperty(
      "checked",
      true,
    );
    fireEvent.click(within(form).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).toBeNull();
    });
    expect(lastBody(server, "POST")).toEqual({
      name: "Ben",
      password: "nordlicht-2026",
      role: "user",
      contentLimit: { preset: "custom", maxAge: 18, hideUnrated: true },
      libraryAccess: { all: false, libraryIds: [shows.id] },
    });
    expect(await screen.findByRole("rowheader", { name: "Ben" })).toBeTruthy();
    expect(screen.getByText("Ben saved.").getAttribute("role")).toBe("status");
  });

  it("edits library access and sends only the change", async () => {
    const server = createFakeUsersServer();
    renderApp(usersPath, { fetch: server.fetch });
    const form = await openEditor("Jonas");

    expect(within(form).getByRole("radio", { name: "Selected libraries" })).toHaveProperty(
      "checked",
      true,
    );
    expect(within(form).getByRole("checkbox", { name: "Movies" })).toHaveProperty("checked", true);
    fireEvent.click(within(form).getByRole("checkbox", { name: "Shows" }));
    fireEvent.click(within(form).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(lastBody(server, "PATCH")).toEqual({
        libraryAccess: { all: false, libraryIds: [movies.id, shows.id] },
      });
    });
    await waitFor(() => {
      expect(screen.queryByRole("dialog")).toBeNull();
    });
    expect(within(rowOf("Jonas")).getByText("2 libraries")).toBeTruthy();
  });

  it("locks the libraries of an admin and enables a disabled account", async () => {
    const server = createFakeUsersServer();
    renderApp(usersPath, { fetch: server.fetch });
    const form = await openEditor("Jonas");

    fireEvent.change(within(form).getByLabelText("Role"), { target: { value: "admin" } });
    expect(within(form).getByRole("radio", { name: "All libraries" })).toHaveProperty(
      "disabled",
      true,
    );
    expect(within(form).getByText("Administrators always see all libraries.")).toBeTruthy();
    fireEvent.click(within(form).getByRole("switch", { name: "Account disabled" }));
    fireEvent.click(within(form).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(lastBody(server, "PATCH")).toEqual({ role: "admin", status: "active" });
    });
  });

  it("shows the last-admin refusal inside the dialog", async () => {
    const server = createFakeUsersServer({ update: () => problemResponse(409, "last_admin") });
    renderApp(usersPath, { fetch: server.fetch });
    const form = await openEditor("Anna");

    fireEvent.change(within(form).getByLabelText("Role"), { target: { value: "user" } });
    fireEvent.click(within(form).getByRole("button", { name: "Save" }));

    expect(
      await within(form).findByText(
        "This is the last active administrator. Make another account an administrator first.",
      ),
    ).toBeTruthy();
    expect(lastBody(server, "PATCH")).toEqual({ role: "user", libraryAccess: { all: true } });
    expect(screen.getByRole("dialog")).toBeTruthy();
  });

  it("shows a taken name next to the name field", async () => {
    const server = createFakeUsersServer({ update: () => problemResponse(409, "name_taken") });
    renderApp(usersPath, { fetch: server.fetch });
    const form = await openEditor("Jonas");

    const name = within(form).getByLabelText("Name");
    fireEvent.change(name, { target: { value: "Anna" } });
    fireEvent.click(within(form).getByRole("button", { name: "Save" }));

    expect(await within(form).findByText("Another account already has this name.")).toBeTruthy();
    expect(name.getAttribute("aria-invalid")).toBe("true");
  });

  it("resets a password with the one Save button", async () => {
    const server = createFakeUsersServer();
    renderApp(usersPath, { fetch: server.fetch });
    const form = await openEditor("Jonas");

    fireEvent.click(within(form).getByRole("button", { name: "Reset password" }));
    const password = within(form).getByLabelText("New password");
    expect(document.activeElement).toBe(password);
    fireEvent.click(within(form).getByRole("button", { name: "Keep the current password" }));
    expect(within(form).queryByLabelText("New password")).toBeNull();
    fireEvent.click(within(form).getByRole("button", { name: "Reset password" }));
    fireEvent.change(within(form).getByLabelText("New password"), {
      target: { value: "winterlicht-2026" },
    });
    fireEvent.click(within(form).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(lastBody(server, "PUT")).toEqual({ password: "winterlicht-2026" });
    });
    expect(server.bodies.some((entry) => entry.method === "PATCH")).toBe(false);
  });

  it("asks before deleting and says that watch state goes too", async () => {
    const server = createFakeUsersServer();
    renderApp(usersPath, { fetch: server.fetch });
    await table();
    fireEvent.click(screen.getByRole("button", { name: "Delete Jonas" }));

    const confirm = dialog();
    expect(confirm.textContent).toContain("their watch state");
    fireEvent.click(within(confirm).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Delete Jonas" }));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Delete account" }));
    await waitFor(() => {
      expect(screen.queryByRole("rowheader", { name: "Jonas" })).toBeNull();
    });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("keeps the confirmation open with the last-admin refusal", async () => {
    const server = createFakeUsersServer({ remove: () => problemResponse(409, "last_admin") });
    renderApp(usersPath, { fetch: server.fetch });
    await table();
    fireEvent.click(screen.getByRole("button", { name: "Delete Anna" }));
    fireEvent.click(within(dialog()).getByRole("button", { name: "Delete account" }));

    expect(await within(dialog()).findByText(/This is the last active administrator/)).toBeTruthy();
  });

  it("shows German strings", async () => {
    renderApp(usersPath, { fetch: createFakeUsersServer().fetch, languages: ["de"] });

    expect(await screen.findByRole("table", { name: "Konten und Profile" })).toBeTruthy();
    expect(screen.getByRole("heading", { level: 1, name: "Benutzer" })).toBeTruthy();
    expect(within(rowOf("Jonas")).getByText("Deaktiviert")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Benutzer hinzufügen" }));
    expect(within(dialog()).getByText("Inhaltsgrenze")).toBeTruthy();
  });
});

describe("Admin › Users visibility", () => {
  it("shows the not-found page and no nav entry to a viewer without admin powers", async () => {
    const server = createFakeUsersServer({}, [
      { method: "GET", path: "/api/v1/me", respond: () => jsonResponse(userMe) },
    ]);
    renderApp(usersPath, { fetch: server.fetch });

    expect(await screen.findByRole("heading", { name: "Page not found" })).toBeTruthy();
    expect(server.countRequests("GET", "/api/v1/users")).toBe(0);
  });

  it("hides the nav entry from a viewer without admin powers", async () => {
    const server = createFakeUsersServer({}, [
      { method: "GET", path: "/api/v1/me", respond: () => jsonResponse(userMe) },
    ]);
    renderApp("/admin/settings/about", { fetch: server.fetch });

    const nav = await screen.findByRole("navigation", { name: "Settings pages" });
    expect(within(nav).queryByRole("link", { name: "Users" })).toBeNull();
    expect(within(nav).queryByRole("link", { name: "Devices" })).toBeNull();
  });

  it("offers the nav entry to an admin", async () => {
    renderApp("/admin/settings/about", { fetch: createFakeUsersServer().fetch });

    const nav = await screen.findByRole("navigation", { name: "Settings pages" });
    expect(within(nav).getByRole("link", { name: "Users" }).getAttribute("href")).toBe(usersPath);
    expect(within(nav).getByRole("link", { name: "Devices" }).getAttribute("href")).toBe(
      "/admin/devices",
    );
  });
});

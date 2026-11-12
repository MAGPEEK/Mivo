import type { Me } from "@mivo/api-client";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../../test/axe";
import { jsonResponse, problemResponse, type FakeRoute } from "../../../../test/fakeApi";
import { renderApp } from "../../../../test/renderApp";
import {
  createFakeUsersServer,
  jonas,
  jonasPrimary,
  max,
  movies,
  shows,
  userMe,
  type FakeUsersOptions,
  type FakeUsersServer,
} from "../../test/fakeUsersServer";

const usersPath = "/admin/users";
const accountPath = "/settings/account";
const jonasProfilesPath = `/api/v1/users/${jonas.id}/profiles`;

/** Jonas signed in with his primary profile: an account owner without admin powers. */
const ownerMe: Me = { ...userMe, profiles: userMe.currentProfile ? [userMe.currentProfile] : [] };

function meRoute(me: () => Me): FakeRoute {
  return { method: "GET", path: "/api/v1/me", respond: () => jsonResponse(me()) };
}

function profilesRoute(respond: () => Response): FakeRoute {
  return { method: "GET", path: "/api/v1/users/:userId/profiles", respond };
}

function ownerServer(options: FakeUsersOptions = {}, me: Me = ownerMe): FakeUsersServer {
  return createFakeUsersServer(options, [meRoute(() => me)]);
}

function dialog(): HTMLElement {
  return screen.getByRole("dialog");
}

function lastBody(server: FakeUsersServer, method: string) {
  return server.bodies.findLast((entry) => entry.method === method);
}

function save(form: HTMLElement): void {
  fireEvent.click(within(form).getByRole("button", { name: "Save" }));
}

/** Opens a dialog from Admin › Users and waits until its libraries, if any, are loaded. */
async function openFromUsers(server: FakeUsersServer, button: string): Promise<HTMLElement> {
  renderApp(usersPath, { fetch: server.fetch });
  fireEvent.click(await screen.findByRole("button", { name: button }));
  const form = dialog();
  await within(form).findByRole("checkbox", { name: "Movies" });
  return form;
}

describe("managed profiles in Admin › Users", () => {
  it("creates a profile with PIN, colour, limit and a subset of the account's libraries", async () => {
    const server = createFakeUsersServer();
    const form = await openFromUsers(server, "Add profile for Jonas");

    expect(
      within(form).getByRole("heading", { name: "Create managed profile for Jonas" }),
    ).toBeTruthy();
    expect(form.textContent).toContain("The limit is enforced on the server");
    expect(form.textContent).not.toContain("Remote access");
    // Jonas only has Movies, so Shows cannot be given to his profiles.
    const showsCheck = within(form).getByRole("checkbox", { name: /^Shows/ });
    expect(showsCheck).toHaveProperty("disabled", true);
    expect(showsCheck.closest("label")?.textContent).toBe("Shows (not available to this account)");
    expect(form.textContent).toContain("A profile sees at most the libraries of its account.");
    expect(await blockingAxeViolations(form)).toEqual([]);

    const pin = within(form).getByLabelText("PIN (optional, 4 digits)");
    fireEvent.change(pin, { target: { value: "12" } });
    save(form);
    expect(within(form).getByText("Enter a name.")).toBeTruthy();
    expect(within(form).getByText("Enter exactly 4 digits.")).toBeTruthy();
    expect(pin.getAttribute("aria-invalid")).toBe("true");
    expect(lastBody(server, "POST")).toBeUndefined();

    fireEvent.change(within(form).getByLabelText("Name"), { target: { value: "Mia" } });
    fireEvent.change(pin, { target: { value: "12x34" } });
    expect(pin).toHaveProperty("value", "1234");
    fireEvent.click(within(form).getByRole("radio", { name: "Teal" }));
    fireEvent.click(within(form).getByRole("radio", { name: "Selected libraries" }));
    fireEvent.click(within(form).getByRole("checkbox", { name: "Movies" }));
    fireEvent.change(within(form).getByLabelText("Preset"), { target: { value: "children" } });
    save(form);

    expect(await screen.findByText("Profile Mia saved.")).toBeTruthy();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(lastBody(server, "POST")).toEqual({
      method: "POST",
      path: jonasProfilesPath,
      body: {
        name: "Mia",
        avatarColor: "teal",
        pin: "1234",
        contentLimit: { preset: "children", maxAge: 6, hideUnrated: true },
        libraryAccess: { all: false, libraryIds: [movies.id] },
      },
    });
  });

  it("shows the profile limit and a taken name of the server", async () => {
    let code = "profile_limit";
    const server = createFakeUsersServer({ createProfile: () => problemResponse(409, code) });
    const form = await openFromUsers(server, "Add profile for Anna");
    fireEvent.change(within(form).getByLabelText("Name"), { target: { value: "Max" } });

    save(form);
    expect(
      await within(form).findByText(
        "This account already has 10 managed profiles. Delete one to add another.",
      ),
    ).toBeTruthy();

    code = "name_taken";
    save(form);
    expect(
      await within(form).findByText("Another profile of this account already has this name."),
    ).toBeTruthy();
    expect(within(form).getByLabelText("Name").getAttribute("aria-invalid")).toBe("true");
  });

  it("changes and removes the PIN of a profile", async () => {
    const server = createFakeUsersServer();
    const form = await openFromUsers(server, "Edit profile Max");
    expect(within(form).getByRole("heading", { name: "Edit profile Max" })).toBeTruthy();
    expect(within(form).getByText("This profile has a PIN.")).toBeTruthy();

    fireEvent.click(within(form).getByRole("button", { name: "Change PIN" }));
    const pin = within(form).getByLabelText("New PIN (4 digits)");
    await waitFor(() => {
      expect(document.activeElement).toBe(pin);
    });
    save(form);
    expect(within(form).getByText("Enter the new PIN, or keep the current one.")).toBeTruthy();

    fireEvent.click(within(form).getByRole("button", { name: "Keep the current PIN" }));
    fireEvent.click(within(form).getByRole("button", { name: "Remove PIN" }));
    expect(within(form).getByText("The PIN is removed when you save.")).toBeTruthy();
    save(form);

    expect(await screen.findByText("Profile Max saved.")).toBeTruthy();
    expect(lastBody(server, "PATCH")).toEqual({
      method: "PATCH",
      path: `/api/v1/profiles/${max.id}`,
      body: { pin: null },
    });
  });

  it("deletes a managed profile only after confirmation", async () => {
    const server = createFakeUsersServer();
    renderApp(usersPath, { fetch: server.fetch });

    fireEvent.click(await screen.findByRole("button", { name: "Delete profile Max" }));
    const question = screen.getByRole("dialog");
    expect(question.textContent).toContain("Deletes the profile Max with its watch state.");
    fireEvent.click(within(question).getByRole("button", { name: "Cancel" }));
    expect(lastBody(server, "DELETE")).toBeUndefined();

    fireEvent.click(screen.getByRole("button", { name: "Delete profile Max" }));
    fireEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "Delete profile" }),
    );

    expect(await screen.findByText("Profile deleted.")).toBeTruthy();
    expect(lastBody(server, "DELETE")?.path).toBe(`/api/v1/profiles/${max.id}`);
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "Edit profile Max" })).toBeNull();
    });
    expect(document.activeElement).toBe(screen.getByRole("heading", { level: 1 }));
  });

  it("keeps the question open and explains a failed deletion", async () => {
    const server = createFakeUsersServer({
      removeProfile: () => problemResponse(404, "not_found"),
    });
    renderApp(usersPath, { fetch: server.fetch });

    fireEvent.click(await screen.findByRole("button", { name: "Delete profile Max" }));
    const question = screen.getByRole("dialog");
    fireEvent.click(within(question).getByRole("button", { name: "Delete profile" }));

    expect(await within(question).findByText("The profile no longer exists.")).toBeTruthy();
  });
});

describe("library load guard", () => {
  it("does not save an account or a profile before the libraries are loaded", async () => {
    let failing = true;
    const server = createFakeUsersServer({
      libraries: () =>
        failing
          ? problemResponse(500, "internal_error")
          : jsonResponse({ libraries: [movies, shows] }),
    });
    renderApp(usersPath, { fetch: server.fetch });

    fireEvent.click(await screen.findByRole("button", { name: "Edit Jonas" }));
    const form = dialog();
    expect(
      await within(form).findByText(
        "The libraries could not be loaded. Saving is possible once they are.",
      ),
    ).toBeTruthy();
    // A demotion always sends the access: without the libraries it would go out empty.
    fireEvent.change(within(form).getByLabelText("Name"), { target: { value: "Jonas B" } });
    const saveButton = within(form).getByRole("button", { name: "Save" });
    expect(saveButton.getAttribute("aria-disabled")).toBe("true");
    fireEvent.click(saveButton);
    expect(lastBody(server, "PATCH")).toBeUndefined();

    failing = false;
    fireEvent.click(within(form).getByRole("button", { name: "Try again" }));
    fireEvent.click(await within(form).findByRole("checkbox", { name: "Shows" }));
    save(form);

    expect(await screen.findByText("Jonas B saved.")).toBeTruthy();
    expect(lastBody(server, "PATCH")?.body).toEqual({
      name: "Jonas B",
      libraryAccess: { all: false, libraryIds: [movies.id, shows.id] },
    });
  });

  it("blocks the profile editor the same way", async () => {
    const server = createFakeUsersServer({
      libraries: () => problemResponse(500, "internal_error"),
    });
    renderApp(usersPath, { fetch: server.fetch });

    fireEvent.click(await screen.findByRole("button", { name: "Add profile for Jonas" }));
    const form = dialog();
    await within(form).findByRole("button", { name: "Try again" });
    fireEvent.change(within(form).getByLabelText("Name"), { target: { value: "Mia" } });
    save(form);

    expect(lastBody(server, "POST")).toBeUndefined();
    expect(within(form).queryByText("Enter a name.")).toBeNull();
  });
});

describe("own account page", () => {
  it("lets the account owner add a profile without the user administration", async () => {
    const server = ownerServer();
    const { router } = renderApp(accountPath, { fetch: server.fetch });

    expect(await screen.findByRole("heading", { level: 1, name: "Your profiles" })).toBeTruthy();
    const list = await screen.findByRole("list", { name: "Profiles of Jonas" });
    expect(within(list).getByText("Primary profile")).toBeTruthy();
    expect(within(list).queryByRole("button", { name: /^Delete profile/ })).toBeNull();
    expect(await blockingAxeViolations(document.body)).toEqual([]);

    fireEvent.click(screen.getByRole("button", { name: "Add profile" }));
    const form = await screen.findByRole("dialog");
    expect(router.state.location.search).toEqual({ profile: "new" });
    expect(within(form).getByRole("heading", { name: "Create managed profile" })).toBeTruthy();
    await within(form).findByRole("checkbox", { name: "Movies" });
    fireEvent.change(within(form).getByLabelText("Name"), { target: { value: "Lena" } });
    save(form);

    expect(await screen.findByText("Profile Lena saved.")).toBeTruthy();
    expect(lastBody(server, "POST")?.path).toBe(jonasProfilesPath);
    expect(await within(list).findByText("Lena")).toBeTruthy();
    expect(within(list).getByRole("button", { name: "Delete profile Lena" })).toBeTruthy();
    expect(router.state.location.search).toEqual({});
    expect(server.countRequests("GET", "/api/v1/users")).toBe(0);
  });

  it("edits only colour and PIN of the own primary profile", async () => {
    const server = ownerServer();
    renderApp(`${accountPath}?profile=${jonasPrimary.id}`, { fetch: server.fetch });

    const form = await screen.findByRole("dialog");
    expect(within(form).getByRole("heading", { name: "Edit profile Jonas" })).toBeTruthy();
    expect(within(form).queryByLabelText("Name")).toBeNull();
    expect(within(form).queryByLabelText("Preset")).toBeNull();
    expect(form.textContent).toContain("This is the primary profile.");
    fireEvent.change(within(form).getByLabelText("PIN (optional, 4 digits)"), {
      target: { value: "2468" },
    });
    fireEvent.click(within(form).getByRole("radio", { name: "Violet" }));
    save(form);

    expect(await screen.findByText("Profile Jonas saved.")).toBeTruthy();
    expect(lastBody(server, "PATCH")).toEqual({
      method: "PATCH",
      path: `/api/v1/profiles/${jonasPrimary.id}`,
      body: { avatarColor: "violet", pin: "2468" },
    });
  });

  it("deletes an own managed profile after confirmation", async () => {
    const server = ownerServer();
    server.profiles.push({ ...max, id: "3c2b1a0f-9e8d-4c7b-8a6f-5e4d3c2b1a0f", userId: jonas.id });
    renderApp(accountPath, { fetch: server.fetch });

    fireEvent.click(await screen.findByRole("button", { name: "Delete profile Max" }));
    fireEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "Delete profile" }),
    );

    expect(await screen.findByText("Profile deleted.")).toBeTruthy();
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "Delete profile Max" })).toBeNull();
    });
  });

  it("tells a managed profile to switch to the primary profile", async () => {
    const managed = { ...max, name: "Lena", isAdmin: false };
    const me: Me = { ...ownerMe, currentProfile: { ...managed, hasPin: false } };
    const server = ownerServer({}, me);
    renderApp(accountPath, { fetch: server.fetch });

    expect(
      await screen.findByText(/Only the primary profile can manage the profiles of this account/),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Add profile" })).toBeNull();
    expect(server.countRequests("GET", jonasProfilesPath)).toBe(0);
  });

  it("explains a refusal of the server", async () => {
    const server = createFakeUsersServer({}, [
      meRoute(() => ownerMe),
      profilesRoute(() => problemResponse(403, "forbidden")),
    ]);
    renderApp(accountPath, { fetch: server.fetch });

    expect((await screen.findByRole("alert")).textContent).toContain(
      "Only the primary profile of the account or an administrator",
    );
  });

  it("retries a failed profile list", async () => {
    let status = 500;
    const server = createFakeUsersServer({}, [
      meRoute(() => ownerMe),
      profilesRoute(() =>
        status === 200
          ? jsonResponse({ profiles: [jonasPrimary] })
          : problemResponse(status, "internal_error"),
      ),
    ]);
    renderApp(accountPath, { fetch: server.fetch });
    const retry = await screen.findByRole("button", { name: "Try again" });
    status = 200;
    fireEvent.click(retry);
    expect(await screen.findByRole("list", { name: "Profiles of Jonas" })).toBeTruthy();
  });

  it("opens the editor from 'Add profile' in the picker", async () => {
    let me: Me = { ...ownerMe, currentProfile: null };
    const server = createFakeUsersServer({}, [
      meRoute(() => me),
      {
        method: "PUT",
        path: "/api/v1/me/profile",
        respond: () => {
          me = ownerMe;
          return jsonResponse(me);
        },
      },
    ]);
    const { router } = renderApp("/profiles", { fetch: server.fetch });

    fireEvent.click(await screen.findByRole("button", { name: "Add profile" }));

    const form = await screen.findByRole("dialog");
    expect(within(form).getByRole("heading", { name: "Create managed profile" })).toBeTruthy();
    expect(router.state.location.pathname).toBe(accountPath);
    expect(router.state.location.search).toEqual({ profile: "new" });
  });
});

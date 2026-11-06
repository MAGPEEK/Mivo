import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import { problemResponse } from "../../../test/fakeApi";
import { renderApp } from "../../../test/renderApp";
import {
  annaProfile,
  createFakeProfilesServer,
  lenaProfile,
  maxProfile,
  type FakeProfilesOptions,
} from "../test/fakeProfilesServer";

async function openMenu(options: FakeProfilesOptions = { currentProfile: annaProfile }) {
  const server = createFakeProfilesServer(options);
  const rendered = renderApp("/libraries/lib-1", { fetch: server.fetch });
  const trigger = await screen.findByRole("button", { name: /^Profile menu: / });
  fireEvent.click(trigger);
  const menu = screen.getByRole("menu", { name: "Profile menu" });
  return { ...rendered, server, trigger, menu };
}

function itemNames(menu: HTMLElement): string[] {
  return within(menu)
    .getAllByRole("menuitem")
    .map((item) => item.textContent);
}

describe("profile menu", () => {
  it("shows the primary profile with its admin marker, the PIN hint and every action", async () => {
    const { menu, trigger } = await openMenu();

    expect(trigger.getAttribute("aria-label")).toBe("Profile menu: Anna");
    const popup = menu.parentElement ?? menu;
    expect(popup.textContent).toContain("Admin");
    expect(popup.textContent).toContain("Primary profile");
    expect(popup.textContent).toContain("Set a PIN on your profile");
    expect(itemNames(menu)).toEqual(["Switch profile", "Change password…", "Devices", "Sign out"]);
    expect(document.activeElement).toBe(within(menu).getAllByRole("menuitem")[0]);
    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });

  it("offers a managed profile no password change and no PIN hint", async () => {
    const { menu } = await openMenu({ currentProfile: lenaProfile });

    const popup = menu.parentElement ?? menu;
    expect(popup.textContent).toContain("Managed profile");
    expect(popup.textContent).not.toContain("Admin");
    expect(popup.textContent).not.toContain("Set a PIN");
    expect(itemNames(menu)).toEqual(["Switch profile", "Devices", "Sign out"]);
  });

  it("switches profile through the picker and back to the page", async () => {
    const { menu, router, server } = await openMenu({
      currentProfile: maxProfile,
      profiles: [annaProfile, lenaProfile, maxProfile],
    });

    fireEvent.click(within(menu).getByRole("menuitem", { name: "Switch profile" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Who's watching?" })).toBeTruthy();
    expect(router.state.location.search).toMatchObject({ next: "/libraries/lib-1" });
    expect(server.countRequests("DELETE", "/api/v1/me/profile")).toBe(1);

    fireEvent.click(
      within(screen.getByRole("list", { name: "Profiles" })).getByRole("button", {
        name: /^Lena/,
      }),
    );
    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/libraries/lib-1");
    });
    expect(await screen.findByRole("button", { name: "Profile menu: Lena" })).toBeTruthy();
  });

  it("opens the password dialog", async () => {
    const { menu } = await openMenu();

    fireEvent.click(within(menu).getByRole("menuitem", { name: "Change password…" }));

    expect(screen.getByRole("dialog", { name: "Change password" })).toBeTruthy();
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("opens the own devices page", async () => {
    const { menu, router } = await openMenu();

    fireEvent.click(within(menu).getByRole("menuitem", { name: "Devices" }));

    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/settings/devices");
    });
  });

  it("signs out", async () => {
    const { menu, router, server } = await openMenu();

    fireEvent.click(within(menu).getByRole("menuitem", { name: "Sign out" }));

    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/sign-in");
    });
    expect(server.countRequests("DELETE", "/api/v1/auth/session")).toBe(1);
  });

  it("says so when switching fails", async () => {
    const { menu } = await openMenu({
      currentProfile: annaProfile,
      routes: [
        {
          method: "DELETE",
          path: "/api/v1/me/profile",
          respond: () => problemResponse(500, "internal_error"),
        },
      ],
    });

    fireEvent.click(within(menu).getByRole("menuitem", { name: "Switch profile" }));

    expect(await screen.findByText("That did not work. Please try again.")).toBeTruthy();
  });

  it("closes on Escape and returns focus to the avatar", async () => {
    const { menu, trigger } = await openMenu();

    fireEvent.keyDown(menu, { key: "Escape" });

    expect(screen.queryByRole("menu")).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });
});

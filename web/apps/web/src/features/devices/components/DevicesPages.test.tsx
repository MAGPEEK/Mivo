import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import { problemResponse, respondNever } from "../../../test/fakeApi";
import { renderApp } from "../../../test/renderApp";
import { createFakeDevicesServer } from "../test/fakeDevicesServer";

const ownPath = "/settings/devices";
const adminPath = "/admin/devices";
const notImplementedStatus = 501;
const forbiddenStatus = 403;
const serverErrorStatus = 500;

async function findTable() {
  return screen.findByRole("table", { name: "Signed-in devices" });
}

function rowOf(table: HTMLElement, deviceName: string): HTMLElement {
  const header = within(table).getByRole("rowheader", { name: new RegExp(deviceName) });
  const row = header.closest("tr");
  if (row === null) {
    throw new Error(`no row for ${deviceName}`);
  }
  return row;
}

describe("OwnDevicesPage", () => {
  it("lists the own devices with client, platform, last seen, network and this-device marker", async () => {
    const server = createFakeDevicesServer();
    renderApp(ownPath, { fetch: server.fetch });

    const table = await findTable();
    const own = rowOf(table, "Firefox on Windows");
    expect(within(own).getByText("This device")).toBeTruthy();
    expect(within(own).getByText("Web")).toBeTruthy();
    expect(within(own).getByText("Windows")).toBeTruthy();
    expect(within(own).getByText("just now")).toBeTruthy();
    expect(within(own).getByText("LAN")).toBeTruthy();
    const tv = rowOf(table, "Living room TV");
    expect(within(tv).getByText("TV app")).toBeTruthy();
    expect(within(tv).getByText("Unknown")).toBeTruthy();
    expect(within(tv).getByText("2 days ago")).toBeTruthy();
    expect(within(tv).getByText("Remote")).toBeTruthy();
    expect(within(table).queryByRole("columnheader", { name: "Account" })).toBeNull();
    expect(screen.getByRole("link", { name: "Devices of all accounts" })).toBeTruthy();
    expect(await blockingAxeViolations(screen.getByRole("main"))).toEqual([]);
  });

  it("revokes another device after confirmation and keeps this one signed in", async () => {
    const server = createFakeDevicesServer();
    renderApp(ownPath, { fetch: server.fetch });

    const table = await findTable();
    fireEvent.click(within(table).getByRole("button", { name: "Revoke Living room TV" }));
    const dialog = screen.getByRole("dialog", { name: "Revoke Living room TV?" });
    expect(within(dialog).getByText(/has to sign in again/)).toBeTruthy();
    fireEvent.click(within(dialog).getByRole("button", { name: "Revoke" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).toBeNull();
    });
    await waitFor(() => {
      expect(screen.queryByText("Living room TV")).toBeNull();
    });
    expect(screen.getByText("Firefox on Windows")).toBeTruthy();
    expect(
      server.countRequests("DELETE", "/api/v1/me/devices/2b3c4d5e-6f70-4a81-9b2c-3d4e5f607182"),
    ).toBe(1);
    expect(server.countRequests("DELETE", "/api/v1/auth/session")).toBe(0);
  });

  it("signs out and opens the sign-in page when this device is revoked", async () => {
    const server = createFakeDevicesServer();
    const { router } = renderApp(ownPath, { fetch: server.fetch });

    const table = await findTable();
    fireEvent.click(within(table).getByRole("button", { name: "Revoke Firefox on Windows" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText(/signs you out here/)).toBeTruthy();
    fireEvent.click(within(dialog).getByRole("button", { name: "Revoke" }));

    expect(await screen.findByText("You have signed out.")).toBeTruthy();
    expect(router.state.location.pathname).toBe("/sign-in");
  });

  it("cancels the confirmation without revoking", async () => {
    const server = createFakeDevicesServer();
    renderApp(ownPath, { fetch: server.fetch });

    const table = await findTable();
    fireEvent.click(within(table).getByRole("button", { name: "Revoke Living room TV" }));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(server.devices).toHaveLength(2);
  });

  it("shows the failure inside the dialog when the revoke fails", async () => {
    const server = createFakeDevicesServer([
      {
        method: "DELETE",
        path: "/api/v1/me/devices/:deviceId",
        respond: () => problemResponse(serverErrorStatus, "internal_error"),
      },
    ]);
    renderApp(ownPath, { fetch: server.fetch });

    const table = await findTable();
    fireEvent.click(within(table).getByRole("button", { name: "Revoke Living room TV" }));
    fireEvent.click(screen.getByRole("button", { name: "Revoke" }));

    expect(await screen.findByText("The device could not be revoked. Try again.")).toBeTruthy();
  });

  it("says the list is not available yet when the server answers 501", async () => {
    const server = createFakeDevicesServer([
      {
        method: "GET",
        path: "/api/v1/me/devices",
        respond: () => problemResponse(notImplementedStatus, "not_implemented"),
      },
    ]);
    renderApp(ownPath, { fetch: server.fetch });

    expect(await screen.findByRole("heading", { name: "Not available yet" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Try again" })).toBeNull();
  });

  it("offers a retry when the server fails", async () => {
    let failing = true;
    const server = createFakeDevicesServer([
      {
        method: "GET",
        path: "/api/v1/me/devices",
        respond: () =>
          failing
            ? problemResponse(serverErrorStatus, "internal_error")
            : new Response(JSON.stringify({ devices: [] }), {
                headers: { "Content-Type": "application/json" },
              }),
      },
    ]);
    renderApp(ownPath, { fetch: server.fetch });

    expect(await screen.findByText("The devices could not be loaded.")).toBeTruthy();
    failing = false;
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByText("No devices are signed in.")).toBeTruthy();
  });

  it("shows the loading state and speaks German", async () => {
    renderApp(ownPath, { fetch: respondNever(), languages: ["de"] });

    expect(await screen.findByRole("heading", { level: 1, name: "Deine Geräte" })).toBeTruthy();
    expect(screen.getByText("Geräte werden geladen")).toBeTruthy();
  });
});

describe("AdminDevicesPage", () => {
  it("lists the devices of every account and filters them by client type", async () => {
    const server = createFakeDevicesServer();
    renderApp(adminPath, { fetch: server.fetch });

    const table = await findTable();
    expect(within(table).getByRole("columnheader", { name: "Account" })).toBeTruthy();
    expect(within(rowOf(table, "Living room TV")).getByText("Jonas")).toBeTruthy();

    const filter = screen.getByRole("combobox", { name: "Client type" });
    fireEvent.change(filter, { target: { value: "tv" } });
    expect(within(table).queryByText("Firefox on Windows")).toBeNull();
    expect(within(table).getByText("Living room TV")).toBeTruthy();

    fireEvent.change(filter, { target: { value: "cli" } });
    expect(screen.getByText("No devices of this client type.")).toBeTruthy();
    expect(await blockingAxeViolations(screen.getByRole("main"))).toEqual([]);
  });

  it("revokes a device of another account through the admin endpoint", async () => {
    const server = createFakeDevicesServer();
    renderApp(adminPath, { fetch: server.fetch });

    const table = await findTable();
    fireEvent.click(within(table).getByRole("button", { name: "Revoke Living room TV" }));
    fireEvent.click(screen.getByRole("button", { name: "Revoke" }));

    await waitFor(() => {
      expect(screen.queryByText("Living room TV")).toBeNull();
    });
    expect(
      server.countRequests("DELETE", "/api/v1/devices/2b3c4d5e-6f70-4a81-9b2c-3d4e5f607182"),
    ).toBe(1);
  });

  it("explains a refused list to a non-admin", async () => {
    const server = createFakeDevicesServer([
      {
        method: "GET",
        path: "/api/v1/devices",
        respond: () => problemResponse(forbiddenStatus, "forbidden"),
      },
    ]);
    renderApp(adminPath, { fetch: server.fetch });

    expect(await screen.findByRole("heading", { name: "Only for administrators" })).toBeTruthy();
  });
});

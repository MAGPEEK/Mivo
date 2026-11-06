import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import { problemResponse } from "../../../test/fakeApi";
import { renderApp } from "../../../test/renderApp";
import {
  createFakeProfilesServer,
  lenaProfile,
  maxPin,
  maxProfile,
  type FakeProfilesServer,
} from "../test/fakeProfilesServer";

const pickerHeading = { level: 1, name: "Who's watching?" } as const;
const homeHeading = { level: 1, name: "Home" } as const;

async function openPicker(server: FakeProfilesServer, path = "/profiles") {
  const rendered = renderApp(path, { fetch: server.fetch });
  await screen.findByRole("heading", pickerHeading);
  return rendered;
}

function tile(name: RegExp) {
  return within(screen.getByRole("list", { name: "Profiles" })).getByRole("button", { name });
}

function pressKey(key: string) {
  const target = document.activeElement ?? document.body;
  fireEvent.keyDown(target, { key });
}

// jsdom does not turn Enter on a focused button into a click; a browser does.
function pressEnter() {
  const target = document.activeElement;
  if (target instanceof HTMLElement) {
    fireEvent.keyDown(target, { key: "Enter" });
    target.click();
  }
}

async function openPinPad(server: FakeProfilesServer) {
  await openPicker(server);
  const max = tile(/^Max/);
  max.focus();
  pressEnter();
  return screen.findByRole("region", { name: "PIN for Max" });
}

describe("profile picker", () => {
  it("shows every profile with its lock and admin markers and no axe violations", async () => {
    const server = createFakeProfilesServer();
    await openPicker(server);

    expect(tile(/^Anna/).textContent).toContain("Admin");
    expect(tile(/^Max/).textContent).toContain("PIN required");
    expect(tile(/^Lena/).textContent).not.toContain("PIN required");
    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });

  it("selects a profile without PIN at once and returns to next", async () => {
    const server = createFakeProfilesServer();
    const { router } = await openPicker(server, "/profiles?next=%2Flibraries%2Flib-1");

    fireEvent.click(tile(/^Lena/));

    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/libraries/lib-1");
    });
    expect(server.state.selections).toEqual([{ profileId: lenaProfile.id }]);
  });

  it("opens Home after the selection when there is no next page", async () => {
    const server = createFakeProfilesServer();
    await openPicker(server);

    fireEvent.click(tile(/^Anna/));

    expect(await screen.findByRole("heading", homeHeading)).toBeTruthy();
  });

  it("takes the PIN from the keyboard alone and submits on the 4th digit", async () => {
    const server = createFakeProfilesServer();
    const panel = await openPinPad(server);

    expect(document.activeElement).toBe(within(panel).getByRole("button", { name: "1" }));
    pressKey("ArrowRight");
    expect(document.activeElement).toBe(within(panel).getByRole("button", { name: "2" }));
    pressKey("ArrowDown");
    expect(document.activeElement).toBe(within(panel).getByRole("button", { name: "5" }));
    pressEnter();
    pressKey("Backspace");
    expect(screen.getByText("0 of 4 digits entered")).toBeTruthy();
    for (const digit of maxPin) {
      pressKey(digit);
    }

    expect(await screen.findByRole("heading", homeHeading)).toBeTruthy();
    expect(server.state.selections).toEqual([
      { profileId: maxProfile.id, pin: maxPin, remember: false },
    ]);
  });

  it("keeps the choice when Remember on this device is ticked", async () => {
    const server = createFakeProfilesServer();
    const panel = await openPinPad(server);
    const remember = within(panel).getByRole("checkbox", { name: "Remember on this device" });

    expect(remember).toHaveProperty("checked", false);
    fireEvent.click(remember);
    for (const digit of maxPin) {
      fireEvent.click(within(panel).getByRole("button", { name: digit }));
    }

    expect(await screen.findByRole("heading", homeHeading)).toBeTruthy();
    expect(server.state.selections).toEqual([
      { profileId: maxProfile.id, pin: maxPin, remember: true },
    ]);
  });

  it("says a PIN is wrong and clears the digits", async () => {
    const server = createFakeProfilesServer();
    const panel = await openPinPad(server);

    for (const digit of "9999") {
      pressKey(digit);
    }

    expect(await within(panel).findByRole("alert")).toHaveProperty(
      "textContent",
      "Wrong PIN. Please try again.",
    );
    expect(screen.getByText("0 of 4 digits entered")).toBeTruthy();
  });

  it("locks the pad with the countdown the server asks for", async () => {
    const server = createFakeProfilesServer();
    server.state.wrongPinsUntilLock = 1;
    server.state.lockoutSeconds = 125;
    const panel = await openPinPad(server);

    for (const digit of "9999") {
      pressKey(digit);
    }

    expect(
      await within(panel).findByText(
        "Too many wrong PINs. The profile is locked. Try again in 2:05.",
        { selector: "[aria-hidden]" },
      ),
    ).toBeTruthy();
    expect(within(panel).getByRole("button", { name: "1" }).getAttribute("aria-disabled")).toBe(
      "true",
    );
    pressKey("1");
    expect(screen.getByText("0 of 4 digits entered")).toBeTruthy();
  });

  it("returns focus to the tile when the PIN pad is left", async () => {
    const server = createFakeProfilesServer();
    const panel = await openPinPad(server);

    fireEvent.click(within(panel).getByRole("button", { name: "Back to profiles" }));

    expect(screen.queryByRole("region", { name: "PIN for Max" })).toBeNull();
    expect(document.activeElement).toBe(tile(/^Max/));
  });

  it("says so when the server cannot select the profile", async () => {
    const server = createFakeProfilesServer({
      routes: [
        {
          method: "PUT",
          path: "/api/v1/me/profile",
          respond: () => problemResponse(500, "internal_error"),
        },
      ],
    });
    await openPicker(server);

    fireEvent.click(tile(/^Lena/));

    expect(await screen.findByText("That did not work. Please try again.")).toBeTruthy();
  });

  it("is never shown when the server has no accounts yet (501)", async () => {
    const server = createFakeProfilesServer({
      routes: [
        {
          method: "GET",
          path: "/api/v1/setup",
          respond: () => problemResponse(501, "not_implemented"),
        },
        {
          method: "GET",
          path: "/api/v1/me",
          respond: () => problemResponse(501, "not_implemented"),
        },
      ],
    });
    const { router } = renderApp("/profiles", { fetch: server.fetch });

    expect(await screen.findByRole("heading", homeHeading)).toBeTruthy();
    expect(router.state.location.pathname).toBe("/");
    expect(screen.queryByRole("button", { name: /^Profile menu/ })).toBeNull();
  });
});

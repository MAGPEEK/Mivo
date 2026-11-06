import { productName } from "@mivo/ui-tokens";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import { problemResponse, respondNeverAtAll, type FakeRoute } from "../../../test/fakeApi";
import { renderApp } from "../../../test/renderApp";
import { createFakeAuthServer, fakeCredentials } from "../test/fakeAuthServer";

function typeInto(label: string, value: string) {
  fireEvent.change(screen.getByLabelText(label), { target: { value } });
}

function submit(name: string) {
  fireEvent.click(screen.getByRole("button", { name }));
}

const homeHeading = { level: 1, name: "Home" } as const;
const signInHeading = `Sign in to ${productName}`;

describe("session guard", () => {
  it("keeps every page back while the session check runs", async () => {
    renderApp("/", { fetch: respondNeverAtAll() });

    expect(await screen.findByRole("status")).toHaveProperty(
      "textContent",
      "Checking your session",
    );
    expect(screen.queryByRole("heading", homeHeading)).toBeNull();
  });

  it("runs without a guard when the server has no accounts yet (501)", async () => {
    const server = createFakeAuthServer({
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
    const { router } = renderApp("/sign-in", { fetch: server.fetch });

    expect(await screen.findByRole("heading", homeHeading)).toBeTruthy();
    expect(router.state.location.pathname).toBe("/");
  });

  it("sends a fresh server to the setup", async () => {
    const server = createFakeAuthServer({ initial: { setupRequired: true, signedIn: false } });
    const { router } = renderApp("/libraries/lib-1", { fetch: server.fetch });

    expect(
      await screen.findByRole("heading", { level: 1, name: "Create admin account" }),
    ).toBeTruthy();
    expect(router.state.location.pathname).toBe("/setup");
  });

  it("sends a visitor without a session to sign-in and back to the page afterwards", async () => {
    const server = createFakeAuthServer({ initial: { signedIn: false } });
    const { router } = renderApp("/items/item-1/play?fileId=file-1", { fetch: server.fetch });

    expect(await screen.findByRole("heading", { level: 1, name: signInHeading })).toBeTruthy();
    expect(router.state.location.pathname).toBe("/sign-in");
    expect(router.state.location.search).toMatchObject({
      next: "/items/item-1/play?fileId=file-1",
    });

    typeInto("Name", ` ${fakeCredentials.name} `);
    typeInto("Password", fakeCredentials.password);
    submit("Sign in");

    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/items/item-1/play");
    });
    expect(router.state.location.search).toMatchObject({ fileId: "file-1" });
    const signIn = server.requests.find((request) => request.method === "POST");
    expect(await signIn?.clone().json()).toEqual({
      name: fakeCredentials.name,
      password: fakeCredentials.password,
      client: "web",
    });
  });

  it("never follows a next that leaves the app", async () => {
    const server = createFakeAuthServer({ initial: { signedIn: false } });
    const { router } = renderApp("/sign-in?next=%2F%2Fevil.example", { fetch: server.fetch });

    await screen.findByRole("heading", { level: 1, name: signInHeading });
    typeInto("Name", fakeCredentials.name);
    typeInto("Password", fakeCredentials.password);
    submit("Sign in");

    expect(await screen.findByRole("heading", homeHeading)).toBeTruthy();
    expect(router.state.location.href).toBe("/");
  });

  it("opens the profile placeholder when the session has no profile", async () => {
    const server = createFakeAuthServer({ initial: { currentProfile: null } });
    const { router } = renderApp("/items/item-1", { fetch: server.fetch });

    expect(await screen.findByRole("heading", { level: 1, name: "Choose a profile" })).toBeTruthy();
    expect(router.state.location.search).toMatchObject({ next: "/items/item-1" });
  });
});

describe("sign-in form", () => {
  async function openSignIn(routes: Parameters<typeof createFakeAuthServer>[0] = {}) {
    const server = createFakeAuthServer({ ...routes, initial: { signedIn: false } });
    const rendered = renderApp("/sign-in", { fetch: server.fetch });
    await screen.findByRole("heading", { level: 1, name: signInHeading });
    return { server, ...rendered };
  }

  it("marks empty fields without asking the server", async () => {
    const { server } = await openSignIn();

    submit("Sign in");

    expect(screen.getByText("Please enter a name.")).toBeTruthy();
    expect(screen.getByText("Please enter your password.")).toBeTruthy();
    expect(screen.getByLabelText("Name").getAttribute("aria-invalid")).toBe("true");
    expect(document.activeElement).toBe(screen.getByLabelText("Name"));
    expect(server.countRequests("POST", "/api/v1/auth/session")).toBe(0);
  });

  it("shows one generic text for wrong credentials and empties the password", async () => {
    await openSignIn();
    typeInto("Name", "Anna");
    typeInto("Password", "wrong password");
    submit("Sign in");

    expect(await screen.findByText("Name or password is incorrect.")).toBeTruthy();
    expect(screen.getByLabelText("Password")).toHaveProperty("value", "");
    expect(document.activeElement).toBe(screen.getByLabelText("Password"));
  });

  it("shows the throttling countdown and blocks the button", async () => {
    await openSignIn({
      routes: [
        {
          method: "POST",
          path: "/api/v1/auth/session",
          respond: () =>
            new Response(
              JSON.stringify({
                type: "about:blank",
                title: "Too many attempts",
                status: 429,
                code: "too_many_attempts",
                retryAfterSeconds: 65,
              }),
              { status: 429, headers: { "Content-Type": "application/problem+json" } },
            ),
        },
      ],
    });
    typeInto("Name", "Anna");
    typeInto("Password", "another guess");
    submit("Sign in");

    // Once visible and once for screen readers, which hear it a single time.
    expect(await screen.findAllByText("Too many failed attempts. Try again in 1:05.")).toHaveLength(
      2,
    );
    expect(screen.getByRole("button", { name: "Sign in" })).toHaveProperty("disabled", true);
  });

  it("reports a server it cannot reach", async () => {
    await openSignIn({
      routes: [
        {
          method: "POST",
          path: "/api/v1/auth/session",
          respond: () => Promise.reject(new TypeError("Failed to fetch")),
        },
      ],
    });
    typeInto("Name", "Anna");
    typeInto("Password", "whatever");
    submit("Sign in");

    expect(await screen.findByText(/The server cannot be reached/u)).toBeTruthy();
  });

  it("toggles the password between hidden and shown", async () => {
    await openSignIn();
    const password = screen.getByLabelText("Password");

    fireEvent.click(screen.getByRole("button", { name: "Show password" }));
    expect(password.getAttribute("type")).toBe("text");
    fireEvent.click(screen.getByRole("button", { name: "Hide password" }));
    expect(password.getAttribute("type")).toBe("password");
  });

  it("explains the recovery and has no serious axe findings", async () => {
    await openSignIn();

    fireEvent.click(screen.getByText("Forgot your password?"));

    expect(screen.getByText(/admin reset-password --name <Name>/u)).toBeTruthy();
    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });

  it("is translated into German", async () => {
    const server = createFakeAuthServer({ initial: { signedIn: false } });
    renderApp("/sign-in", { fetch: server.fetch, languages: ["de"] });

    expect(
      await screen.findByRole("heading", { level: 1, name: `Bei ${productName} anmelden` }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Anmelden" })).toBeTruthy();
  });
});

describe("first-run setup", () => {
  async function openSetup(routes: readonly FakeRoute[] = []) {
    const server = createFakeAuthServer({
      routes,
      initial: { setupRequired: true, signedIn: false },
    });
    const rendered = renderApp("/setup", { fetch: server.fetch });
    await screen.findByRole("heading", { level: 1, name: "Create admin account" });
    return { server, ...rendered };
  }

  it("checks name, length and repetition before sending", async () => {
    const { server } = await openSetup();
    typeInto("Name", "a".repeat(41));
    typeInto("Password", "short");
    submit("Create account and sign in");

    expect(screen.getByText("At most 40 characters.")).toBeTruthy();
    expect(screen.getByText("At least 8 characters.")).toBeTruthy();
    expect(screen.getByText("Please check the marked fields.")).toBeTruthy();

    typeInto("Name", "Anna");
    typeInto("Password", "a long password");
    typeInto("Repeat password", "a different one");
    submit("Create account and sign in");

    expect(screen.getByText("The passwords do not match.")).toBeTruthy();
    expect(document.activeElement).toBe(screen.getByLabelText("Repeat password"));
    expect(server.countRequests("POST", "/api/v1/setup")).toBe(0);
  });

  it("creates the admin and continues with No libraries yet", async () => {
    const { router } = await openSetup();
    typeInto("Name", "Anna");
    typeInto("Password", "a long password");
    typeInto("Repeat password", "a long password");
    submit("Create account and sign in");

    expect(await screen.findByRole("heading", homeHeading)).toBeTruthy();
    expect(router.state.location.pathname).toBe("/");
    expect((await screen.findAllByText("No libraries yet")).length).toBeGreaterThan(0);
  });

  it("explains that a remote peer cannot run the setup", async () => {
    const { router } = await openSetup([
      { method: "POST", path: "/api/v1/setup", respond: () => problemResponse(403, "forbidden") },
    ]);
    typeInto("Name", "Anna");
    typeInto("Password", "a long password");
    typeInto("Repeat password", "a long password");
    submit("Create account and sign in");

    expect(
      await screen.findByRole("heading", { level: 1, name: "Setup only from the local network" }),
    ).toBeTruthy();
    expect(router.state.location.pathname).toBe("/setup/remote");

    submit("Check again");

    expect(
      await screen.findByText("This connection still does not come from the local network."),
    ).toBeTruthy();
  });

  it("returns to the form when the connection is local again", async () => {
    const server = createFakeAuthServer({
      routes: [
        { method: "POST", path: "/api/v1/setup", respond: () => problemResponse(400, "invalid") },
      ],
      initial: { setupRequired: true, signedIn: false },
    });
    const { router } = renderApp("/setup/remote", { fetch: server.fetch });

    fireEvent.click(await screen.findByRole("button", { name: "Check again" }));

    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/setup");
    });
  });

  it("shows the field the server refused", async () => {
    await openSetup([
      {
        method: "POST",
        path: "/api/v1/setup",
        respond: () =>
          new Response(
            JSON.stringify({
              type: "about:blank",
              title: "Invalid",
              status: 400,
              code: "invalid",
              field: "name",
            }),
            { status: 400, headers: { "Content-Type": "application/problem+json" } },
          ),
      },
    ]);
    typeInto("Name", "Anna");
    typeInto("Password", "a long password");
    typeInto("Repeat password", "a long password");
    submit("Create account and sign in");

    expect(await screen.findByText("This name cannot be used.")).toBeTruthy();
  });
});

describe("lost session", () => {
  it("clears the data and opens sign-in with the notice, the name and the way back", async () => {
    const server = createFakeAuthServer({
      routes: [
        {
          method: "GET",
          path: "/api/v1/libraries/:libraryId",
          respond: () => {
            server.state.signedIn = false;
            return problemResponse(401, "unauthenticated");
          },
        },
      ],
    });
    const { router } = renderApp("/libraries/lib-1", { fetch: server.fetch });

    expect(await screen.findByText(/Your session has expired or was ended/u)).toBeTruthy();
    expect(router.state.location.pathname).toBe("/sign-in");
    expect(router.state.location.search).toMatchObject({
      next: "/libraries/lib-1",
      reason: "expired",
    });
    expect(screen.getByLabelText("Name")).toHaveProperty("value", "Anna");
  });

  it("asks for a profile after 403 profile_required", async () => {
    const server = createFakeAuthServer({
      routes: [
        {
          method: "GET",
          path: "/api/v1/libraries/:libraryId",
          respond: () => {
            server.state.currentProfile = null;
            return problemResponse(403, "profile_required");
          },
        },
      ],
    });
    const { router } = renderApp("/libraries/lib-1", { fetch: server.fetch });

    expect(await screen.findByRole("heading", { level: 1, name: "Choose a profile" })).toBeTruthy();
    expect(router.state.location.search).toMatchObject({ next: "/libraries/lib-1" });
  });
});

describe("sign out", () => {
  it("ends the session and says so on the sign-in page", async () => {
    const server = createFakeAuthServer({ initial: { currentProfile: null } });
    const { router } = renderApp("/profiles", { fetch: server.fetch });
    const page = await screen.findByRole("region", { name: "Choose a profile" });

    fireEvent.click(within(page).getByRole("button", { name: "Sign out" }));

    expect(await screen.findByText("You have signed out.")).toBeTruthy();
    expect(router.state.location.search).toMatchObject({ reason: "signed-out" });
    expect(server.countRequests("DELETE", "/api/v1/auth/session")).toBe(1);
  });

  it("stays signed in and says so when the server fails", async () => {
    const server = createFakeAuthServer({
      initial: { currentProfile: null },
      routes: [
        {
          method: "DELETE",
          path: "/api/v1/auth/session",
          respond: () => problemResponse(500, "internal_error"),
        },
      ],
    });
    renderApp("/profiles", { fetch: server.fetch });

    fireEvent.click(await screen.findByRole("button", { name: "Sign out" }));

    expect(await screen.findByText("Signing out did not work. Please try again.")).toBeTruthy();
  });
});

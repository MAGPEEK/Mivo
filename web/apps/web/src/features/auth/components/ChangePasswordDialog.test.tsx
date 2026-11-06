import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { I18nextProvider } from "react-i18next";
import { describe, expect, it, vi } from "vitest";

import { createAppI18n } from "../../../app/i18n";
import { ApiClientProvider } from "../../../shared/api";
import { blockingAxeViolations } from "../../../test/axe";
import {
  createFakeApiClient,
  createFakeServer,
  jsonResponse,
  problemResponse,
  type FakeServer,
} from "../../../test/fakeApi";
import { ChangePasswordDialog } from "./ChangePasswordDialog";

function renderDialog(answer: () => Response, languages = ["en"]) {
  const server = createFakeServer([
    { method: "PUT", path: "/api/v1/me/password", respond: answer },
  ]);
  const onClose = vi.fn();
  render(
    <I18nextProvider i18n={createAppI18n(languages)}>
      <ApiClientProvider client={createFakeApiClient(server.fetch)}>
        <QueryClientProvider client={new QueryClient()}>
          <ChangePasswordDialog onClose={onClose} />
        </QueryClientProvider>
      </ApiClientProvider>
    </I18nextProvider>,
  );
  return { server, onClose };
}

function fill(current: string, next: string, repeat = next) {
  fireEvent.change(screen.getByLabelText("Current password"), { target: { value: current } });
  fireEvent.change(screen.getByLabelText("New password"), { target: { value: next } });
  fireEvent.change(screen.getByLabelText("Repeat new password"), { target: { value: repeat } });
  fireEvent.click(screen.getByRole("button", { name: "Change password" }));
}

async function sentBody(server: FakeServer): Promise<unknown> {
  return server.requests
    .find((request) => request.method === "PUT")
    ?.clone()
    .json();
}

const noContent = () => new Response(null, { status: 204 });

describe("ChangePasswordDialog", () => {
  it("checks the fields before sending", () => {
    const { server } = renderDialog(noContent);

    fill("", "short", "short");

    expect(screen.getByText("Please enter your password.")).toBeTruthy();
    expect(screen.getByText("At least 8 characters.")).toBeTruthy();
    expect(document.activeElement).toBe(screen.getByLabelText("Current password"));
    expect(server.countRequests("PUT", "/api/v1/me/password")).toBe(0);
  });

  it("changes the password and says the other devices were signed out", async () => {
    const { server, onClose } = renderDialog(noContent);

    fill("old password", "a new password");

    expect(await screen.findByText(/Password changed/u)).toBeTruthy();
    expect(await sentBody(server)).toEqual({
      currentPassword: "old password",
      newPassword: "a new password",
    });
    fireEvent.click(screen.getAllByRole("button", { name: "Close" }).at(-1) ?? document.body);
    expect(onClose).toHaveBeenCalled();
  });

  it("marks a wrong current password (403) and stays open", async () => {
    renderDialog(() => problemResponse(403, "invalid_credentials"));

    fill("wrong password", "a new password");

    expect(await screen.findByText("The current password is incorrect.")).toBeTruthy();
    expect(screen.getByRole("dialog", { name: "Change password" })).toBeTruthy();
    expect(screen.getByLabelText("Current password").getAttribute("aria-invalid")).toBe("true");
  });

  it("shows the wait after too many attempts", async () => {
    renderDialog(() =>
      jsonResponse(
        {
          type: "about:blank",
          title: "x",
          status: 429,
          code: "too_many_attempts",
          retryAfterSeconds: 120,
        },
        429,
        "application/problem+json",
      ),
    );

    fill("old password", "a new password");

    expect(await screen.findByText("Too many failed attempts. Try again in 2:00.")).toBeTruthy();
  });

  it("marks a new password the server refused", async () => {
    renderDialog(() =>
      jsonResponse(
        { type: "about:blank", title: "x", status: 400, code: "invalid", field: "newPassword" },
        400,
        "application/problem+json",
      ),
    );

    fill("old password", "a new password");

    expect(await screen.findByText("This password cannot be used.")).toBeTruthy();
  });

  it("reports other failures generically", async () => {
    renderDialog(() => problemResponse(500, "internal_error"));

    fill("old password", "a new password");

    expect(await screen.findByText("That did not work. Please try again.")).toBeTruthy();
  });

  it("has no serious axe findings and a German version", async () => {
    renderDialog(noContent, ["de"]);

    expect(screen.getByRole("dialog", { name: "Passwort ändern" })).toBeTruthy();
    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });
});

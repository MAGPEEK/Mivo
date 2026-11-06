import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from "@tanstack/react-router";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { I18nextProvider } from "react-i18next";
import { describe, expect, it } from "vitest";

import { createAppI18n } from "../../../app/i18n";
import { ApiClientProvider } from "../../../shared/api";
import { createFakeApiClient } from "../../../test/fakeApi";
import { SessionGate } from "../../auth";
import {
  annaProfile,
  createFakeProfilesServer,
  type FakeProfilesServer,
} from "../test/fakeProfilesServer";
import { ProfilePickerPage } from "./ProfilePickerPage";

const editorPath = "/editor";

// The app router passes no editor path until M7-33 adds the editor, so this test builds a
// small router of its own: the session gate, the picker with the path and a stand-in editor.
function renderPicker(server: FakeProfilesServer) {
  const rootRoute = createRootRoute({
    component: () => (
      <SessionGate>
        <Outlet />
      </SessionGate>
    ),
  });
  const routeTree = rootRoute.addChildren([
    createRoute({
      getParentRoute: () => rootRoute,
      path: "/profiles",
      component: () => <ProfilePickerPage addProfilePath={editorPath} />,
    }),
    createRoute({
      getParentRoute: () => rootRoute,
      path: editorPath,
      component: () => <h1>Editor</h1>,
    }),
  ]);
  const router = createRouter({
    routeTree,
    history: createMemoryHistory({ initialEntries: ["/profiles"] }),
  });
  render(
    <I18nextProvider i18n={createAppI18n(["en"])}>
      <ApiClientProvider client={createFakeApiClient(server.fetch)}>
        <QueryClientProvider client={new QueryClient()}>
          <RouterProvider router={router} />
        </QueryClientProvider>
      </ApiClientProvider>
    </I18nextProvider>,
  );
  return router;
}

describe("Add profile", () => {
  it("selects the primary profile and then opens the editor", async () => {
    const server = createFakeProfilesServer();
    const router = renderPicker(server);

    fireEvent.click(await screen.findByRole("button", { name: "Add profile" }));

    expect(await screen.findByRole("heading", { name: "Editor" })).toBeTruthy();
    expect(router.state.location.pathname).toBe(editorPath);
    expect(server.state.selections).toEqual([{ profileId: annaProfile.id }]);
  });

  it("asks for the primary profile's PIN first", async () => {
    const lockedAnna = { ...annaProfile, hasPin: true };
    const server = createFakeProfilesServer({ profiles: [lockedAnna] });
    renderPicker(server);

    fireEvent.click(await screen.findByRole("button", { name: "Add profile" }));

    const panel = await screen.findByRole("region", { name: "PIN for Anna" });
    expect(within(panel).getByRole("checkbox")).toHaveProperty("checked", false);
    await waitFor(() => {
      expect(server.state.selections).toEqual([]);
    });
  });
});

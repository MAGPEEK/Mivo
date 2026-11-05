import type { HomeRows } from "@mivo/api-client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nextProvider } from "react-i18next";
import { describe, expect, it } from "vitest";

import { ApiClientProvider } from "../../../shared/api";
import { createI18n } from "../../../shared/i18n";
import { createFakeApiClient, createFakeServer, type FakeRoute } from "../../../test/fakeApi";
import { librariesRoute } from "../../../test/libraryFixtures";
import { libraryLayoutStorageKey } from "../../sidebar-layout";
import {
  failingHomeRowsRoute,
  heroMovie,
  homeRowsRoute,
  moviesLibrary,
  moviesRow,
  rowsWithHero,
  showsLibrary,
  showsRow,
  twoLibraryRows,
} from "../test/homeFixtures";
import { useHomeRows } from "./useHomeRows";

function renderHomeRows(routes: readonly FakeRoute[]) {
  const server = createFakeServer(routes);
  const apiClient = createFakeApiClient(server.fetch);
  const queryClient = new QueryClient({ defaultOptions: { queries: { retryDelay: 0 } } });
  const i18n = createI18n(["en"]);
  const wrapper = ({ children }: { readonly children: ReactNode }) => (
    <I18nextProvider i18n={i18n}>
      <ApiClientProvider client={apiClient}>
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      </ApiClientProvider>
    </I18nextProvider>
  );
  return { ...renderHook(() => useHomeRows(), { wrapper }), server };
}

const bothLibraries = librariesRoute([moviesLibrary, showsLibrary]);

async function loadedState(routes: readonly FakeRoute[]) {
  const { result } = renderHomeRows(routes);
  await waitFor(() => {
    expect(result.current.state).toBe("loaded");
  });
  return result;
}

describe("useHomeRows", () => {
  it("starts loading", () => {
    const { result } = renderHomeRows([bothLibraries, homeRowsRoute(twoLibraryRows)]);

    expect(result.current.state).toBe("loading");
  });

  it("delivers the server rows in order without a hint", async () => {
    const result = await loadedState([bothLibraries, homeRowsRoute(twoLibraryRows)]);

    expect(result.current).toEqual({
      state: "loaded",
      hero: [],
      rows: [moviesRow, showsRow],
      hint: undefined,
    });
  });

  it("leaves out rows of unpinned libraries", async () => {
    window.localStorage.setItem(
      libraryLayoutStorageKey,
      JSON.stringify({ version: 1, order: [], unpinned: [showsLibrary.id] }),
    );

    const result = await loadedState([bothLibraries, homeRowsRoute(twoLibraryRows)]);

    expect(result.current).toEqual({
      state: "loaded",
      hero: [],
      rows: [moviesRow],
      hint: undefined,
    });
  });

  it("leaves out hero titles of unpinned libraries", async () => {
    window.localStorage.setItem(
      libraryLayoutStorageKey,
      JSON.stringify({ version: 1, order: [], unpinned: [showsLibrary.id] }),
    );

    const result = await loadedState([bothLibraries, homeRowsRoute(rowsWithHero)]);

    expect(result.current).toMatchObject({ state: "loaded", hero: [heroMovie] });
  });

  it("orders the rows as the sidebar", async () => {
    window.localStorage.setItem(
      libraryLayoutStorageKey,
      JSON.stringify({ version: 1, order: [showsLibrary.id, moviesLibrary.id], unpinned: [] }),
    );

    const result = await loadedState([bothLibraries, homeRowsRoute(twoLibraryRows)]);

    expect(result.current).toMatchObject({ state: "loaded", rows: [showsRow, moviesRow] });
  });

  it("hints at the sidebar when every library is unpinned", async () => {
    window.localStorage.setItem(
      libraryLayoutStorageKey,
      JSON.stringify({ version: 1, order: [], unpinned: [moviesLibrary.id, showsLibrary.id] }),
    );

    const result = await loadedState([bothLibraries, homeRowsRoute(twoLibraryRows)]);

    expect(result.current).toEqual({ state: "loaded", hero: [], rows: [], hint: "allUnpinned" });
  });

  it("reports no libraries from the library list", async () => {
    const { result } = renderHomeRows([homeRowsRoute(twoLibraryRows)]);

    await waitFor(() => {
      expect(result.current.state).toBe("noLibraries");
    });
  });

  it("reports no libraries from the server hint", async () => {
    const noLibraries: HomeRows = { rows: [], hero: [], hint: "noLibraries" };
    const { result } = renderHomeRows([bothLibraries, homeRowsRoute(noLibraries)]);

    await waitFor(() => {
      expect(result.current.state).toBe("noLibraries");
    });
  });

  it("tolerates a body without a rows list", async () => {
    const broken = { hero: [] } as unknown as HomeRows;

    const result = await loadedState([bothLibraries, homeRowsRoute(broken)]);

    expect(result.current).toEqual({ state: "loaded", hero: [], rows: [], hint: "generic" });
  });

  it("fails after a retry and loads again on request", async () => {
    let healed = false;
    const { result, server } = renderHomeRows([
      bothLibraries,
      failingHomeRowsRoute(twoLibraryRows, () => healed),
    ]);
    await waitFor(() => {
      expect(result.current.state).toBe("error");
    });
    expect(server.countRequests("GET", "/api/v1/home/rows")).toBe(2);

    healed = true;
    act(() => {
      if (result.current.state === "error") {
        result.current.retry();
      }
    });

    await waitFor(() => {
      expect(result.current.state).toBe("loaded");
    });
  });
});

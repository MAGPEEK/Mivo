import type { Library } from "@mivo/api-client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";

import { ApiClientProvider } from "../../../shared/api";
import { createFakeApiClient, createFakeServer, problemResponse } from "../../../test/fakeApi";
import { fakeLibrary, finishedScan, runningScan } from "../../../test/libraryFixtures";
import { useLibraryScan } from "./useLibraryScan";

function renderScan(initialLibrary: Library) {
  const server = createFakeServer([
    {
      method: "POST",
      path: `/api/v1/libraries/${initialLibrary.id}/scan`,
      respond: () => problemResponse(503, "unavailable"),
    },
  ]);
  const apiClient = createFakeApiClient(server.fetch);
  const queryClient = new QueryClient();
  const wrapper = ({ children }: { readonly children: ReactNode }) => (
    <ApiClientProvider client={apiClient}>
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    </ApiClientProvider>
  );
  const rendered = renderHook(({ library }: { library: Library }) => useLibraryScan(library), {
    wrapper,
    initialProps: { library: initialLibrary },
  });
  return { ...rendered, server };
}

async function failedStart(library: Library) {
  const rendered = renderScan(library);
  act(() => {
    rendered.result.current.start();
  });
  await waitFor(() => {
    expect(rendered.result.current.startError).not.toBeNull();
  });
  return rendered;
}

describe("useLibraryScan start errors", () => {
  it("drops a start error once a scan runs", async () => {
    const library = fakeLibrary();
    const { result, rerender } = await failedStart(library);

    rerender({ library: { ...library, scan: runningScan } });

    expect(result.current.startError).toBeNull();
    expect(result.current.running).toBe(true);
  });

  it("drops a start error once the library reports another scan", async () => {
    const library = fakeLibrary();
    const { result, rerender } = await failedStart(library);

    rerender({ library: { ...library, scan: finishedScan } });

    expect(result.current.startError).toBeNull();
  });

  it("drops a start error when a scan started after an earlier one", async () => {
    const library = fakeLibrary({ scan: finishedScan });
    const { result, rerender } = await failedStart(library);
    // Any other start time counts, earlier or later: server and client clocks may differ.
    const otherScan = { ...finishedScan, startedAt: "2026-10-30T17:00:00Z" };

    rerender({ library: { ...library, scan: otherScan } });

    expect(result.current.startError).toBeNull();
  });

  it("keeps a start error while the library reports the same scan as at the click", async () => {
    const library = fakeLibrary({ scan: finishedScan });
    const { result, rerender } = await failedStart(library);

    rerender({ library: { ...library, scan: { ...finishedScan } } });

    expect(result.current.startError).not.toBeNull();
  });

  it("ignores start while a scan runs", () => {
    const { result, server } = renderScan(fakeLibrary({ scan: runningScan }));

    act(() => {
      result.current.start();
    });

    expect(server.requests).toHaveLength(0);
  });
});

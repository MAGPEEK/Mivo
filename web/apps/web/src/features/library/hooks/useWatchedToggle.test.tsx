import type { ItemDetail, ViewerState } from "@mivo/api-client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { ApiClientProvider, itemQueryKey, itemsQueryKey } from "../../../shared/api";
import { createFakeApiClient, createFakeServer, problemResponse } from "../../../test/fakeApi";
import { librariesQueryKey } from "../api/libraryQueries";
import { useWatchedToggle, type WatchedToggleItem } from "./useWatchedToggle";

const itemId = "movie-1";
const watchedPath = `/api/v1/items/${itemId}/watched`;

const inProgress: ViewerState = { watched: false, positionSeconds: 120, playCount: 0 };
const movie: WatchedToggleItem = { id: itemId, kind: "movie", viewerState: inProgress };
const detail: ItemDetail = {
  id: itemId,
  libraryId: "library-1",
  kind: "movie",
  title: "Big Buck Bunny",
  files: [],
  viewerState: inProgress,
};

type Respond = () => Response | Promise<Response>;

const noContent: Respond = () => new Response(null, { status: 204 });

/** An answer the test sends when it is ready, so the pending state can be looked at. */
function deferredAnswer(): { readonly respond: Respond; readonly send: (r: Response) => void } {
  let send: (response: Response) => void = () => undefined;
  const answer = new Promise<Response>((resolve) => {
    send = resolve;
  });
  return {
    respond: () => answer,
    send: (response) => {
      send(response);
    },
  };
}

// The stub server: the real one answers 501 for these operations until M7-23.
function renderToggle(respond: Respond, item: WatchedToggleItem = movie) {
  const server = createFakeServer([
    { method: "PUT", path: "/api/v1/items/:itemId/watched", respond },
    { method: "DELETE", path: "/api/v1/items/:itemId/watched", respond },
  ]);
  const apiClient = createFakeApiClient(server.fetch);
  const queryClient = new QueryClient();
  queryClient.setQueryData(itemQueryKey(itemId), detail);
  const wrapper = ({ children }: { readonly children: ReactNode }) => (
    <ApiClientProvider client={apiClient}>
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    </ApiClientProvider>
  );
  const rendered = renderHook(() => useWatchedToggle(item), { wrapper });
  const cachedState = () => queryClient.getQueryData<ItemDetail>(itemQueryKey(itemId))?.viewerState;
  return { ...rendered, server, queryClient, wrapper, cachedState };
}

describe("useWatchedToggle", () => {
  it.each([
    ["a watched show", "show", true, true],
    ["a watched season", "season", true, true],
    ["an unwatched show", "show", false, false],
    ["a watched movie", "movie", true, false],
  ])("needs a confirmation to unmark: %s -> %s", (_name, kind, watched, expected) => {
    const { result } = renderToggle(noContent, {
      id: itemId,
      kind,
      viewerState: { watched, positionSeconds: 0, playCount: 1 },
    });

    expect(result.current.confirmUnmark).toBe(expected);
  });

  it("shows the expected state at once and reloads lists, rows and items after the answer", async () => {
    const answer = deferredAnswer();
    const { result, server, queryClient, cachedState } = renderToggle(answer.respond);
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");

    act(() => {
      result.current.toggle();
    });
    await waitFor(() => {
      expect(server.countRequests("PUT", watchedPath)).toBe(1);
    });

    expect(result.current).toMatchObject({ watched: true, pending: true, failure: "none" });
    expect(result.current.viewerState).toEqual({ watched: true, positionSeconds: 0, playCount: 0 });
    expect(cachedState()).toEqual({ watched: true, positionSeconds: 0, playCount: 0 });
    expect(invalidate).not.toHaveBeenCalled();

    act(() => {
      answer.send(new Response(null, { status: 204 }));
    });
    await waitFor(() => {
      expect(result.current.pending).toBe(false);
    });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: librariesQueryKey });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: itemsQueryKey });
  });

  it("ignores a second press while the request runs", async () => {
    const answer = deferredAnswer();
    const { result, server } = renderToggle(answer.respond);

    act(() => {
      result.current.toggle();
    });
    await waitFor(() => {
      expect(result.current.pending).toBe(true);
    });
    act(() => {
      result.current.toggle();
    });

    expect(server.countRequests("PUT", watchedPath)).toBe(1);
    expect(server.countRequests("DELETE", watchedPath)).toBe(0);
  });

  it("marks a watched item unwatched with DELETE", async () => {
    const watched: ViewerState = { watched: true, positionSeconds: 0, playCount: 1 };
    const { result, server } = renderToggle(noContent, { ...movie, viewerState: watched });

    expect(result.current.watched).toBe(true);
    act(() => {
      result.current.toggle();
    });

    await waitFor(() => {
      expect(server.countRequests("DELETE", watchedPath)).toBe(1);
    });
  });

  it("takes the expected state back when the request fails, and can be pressed again", async () => {
    const { result, server, cachedState } = renderToggle(() => problemResponse(500, "internal"));

    act(() => {
      result.current.toggle();
    });
    await waitFor(() => {
      expect(result.current.failure).toBe("failed");
    });

    expect(result.current).toMatchObject({ watched: false, pending: false });
    expect(result.current.viewerState).toEqual(inProgress);
    expect(cachedState()).toEqual(inProgress);

    act(() => {
      result.current.toggle();
    });
    await waitFor(() => {
      expect(server.countRequests("PUT", watchedPath)).toBe(2);
    });
  });

  it("turns itself off for every toggle once the server answers 501", async () => {
    const { result, server, wrapper, cachedState } = renderToggle(() =>
      problemResponse(501, "not_implemented"),
    );

    act(() => {
      result.current.toggle();
    });
    await waitFor(() => {
      expect(result.current.failure).toBe("unsupported");
    });
    expect(result.current.watched).toBe(false);
    expect(cachedState()).toEqual(inProgress);

    act(() => {
      result.current.toggle();
    });
    const other = renderHook(() => useWatchedToggle({ id: "movie-2", kind: "movie" }), { wrapper });

    expect(server.countRequests("PUT", watchedPath)).toBe(1);
    expect(other.result.current.failure).toBe("unsupported");
  });

  it.each([
    ["movie", true],
    ["episode", true],
    ["show", true],
    ["season", true],
    ["extra", false],
    ["album", false],
    ["photo", false],
  ])("is available for a %s: %s", (kind, available) => {
    const { result } = renderToggle(noContent, { id: itemId, kind });

    expect(result.current.available).toBe(available);
    expect(result.current.watched).toBe(false);
    expect(result.current.viewerState).toBeUndefined();
  });
});

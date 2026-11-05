import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ApiClientProvider } from "../../../../shared/api";
import {
  createFakeApiClient,
  createFakeServer,
  jsonResponse,
  problemResponse,
  type FakeServer,
} from "../../../../test/fakeApi";
import { metadataStatusPollMilliseconds, type MetadataStatus } from "../api/metadataSettings";
import { useMetadataStatus } from "./useMetadataStatus";

const statusPath = "/api/v1/metadata/status";
const notImplementedStatus = 501;
const idleStatus: MetadataStatus = { queueLength: 0, providers: [] };

function statusServer(respond: () => Response): FakeServer {
  return createFakeServer([{ method: "GET", path: statusPath, respond }]);
}

function renderStatus(server: FakeServer) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retryDelay: 0 } } });
  const wrapper = ({ children }: { readonly children: ReactNode }) => (
    <ApiClientProvider client={createFakeApiClient(server.fetch)}>
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    </ApiClientProvider>
  );
  return renderHook(() => useMetadataStatus(), { wrapper });
}

// Advances the fake clock and lets the queries' promises settle.
async function advance(milliseconds: number): Promise<void> {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(milliseconds);
  });
}

describe("useMetadataStatus polling", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("asks again every 5 s and stops when unmounted", async () => {
    const server = statusServer(() => jsonResponse(idleStatus));
    const rendered = renderStatus(server);
    await advance(0);

    expect(rendered.result.current.status).toBe("ready");
    expect(server.countRequests("GET", statusPath)).toBe(1);
    await advance(metadataStatusPollMilliseconds);
    expect(server.countRequests("GET", statusPath)).toBe(2);
    await advance(metadataStatusPollMilliseconds);
    expect(server.countRequests("GET", statusPath)).toBe(3);

    rendered.unmount();
    await advance(metadataStatusPollMilliseconds * 3);
    expect(server.countRequests("GET", statusPath)).toBe(3);
  });

  it("stops polling once the server answers 501", async () => {
    const server = statusServer(() => problemResponse(notImplementedStatus, "not_implemented"));
    const rendered = renderStatus(server);
    await advance(0);

    expect(rendered.result.current.status).toBe("unavailable");
    await advance(metadataStatusPollMilliseconds * 3);
    expect(server.countRequests("GET", statusPath)).toBe(1);
  });
});

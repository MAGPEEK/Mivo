import { ApiError, clientErrorCodes } from "@mivo/api-client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";

import { ApiClientProvider } from "../../../shared/api";
import {
  createFakeApiClient,
  fakeSystemInfo,
  respondWithJson,
  type FakeFetch,
} from "../../../test/fakeApi";
import { useSystemInfo } from "./useSystemInfo";

interface Harness {
  readonly queryClient: QueryClient;
  readonly requests: readonly Request[];
  readonly wrapper: (props: { readonly children: ReactNode }) => ReactNode;
}

// Real providers with a fake server; retries run without delay to keep the tests fast.
function createHarness(fetch: FakeFetch): Harness {
  const requests: Request[] = [];
  const apiClient = createFakeApiClient((request) => {
    requests.push(request);
    return fetch(request);
  });
  const queryClient = new QueryClient({ defaultOptions: { queries: { retryDelay: 0 } } });
  const wrapper = ({ children }: { readonly children: ReactNode }) => (
    <ApiClientProvider client={apiClient}>
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    </ApiClientProvider>
  );
  return { queryClient, requests, wrapper };
}

describe("useSystemInfo", () => {
  it("returns the system info of the server", async () => {
    const harness = createHarness(respondWithJson(fakeSystemInfo, 200));

    const { result } = renderHook(useSystemInfo, { wrapper: harness.wrapper });

    await waitFor(() => {
      expect(result.current.data).toEqual(fakeSystemInfo);
    });
    expect(harness.requests[0]?.method).toBe("GET");
  });

  it("keeps the loaded info fresh instead of refetching on the next mount", async () => {
    const harness = createHarness(respondWithJson(fakeSystemInfo, 200));
    const first = renderHook(useSystemInfo, { wrapper: harness.wrapper });
    await waitFor(() => {
      expect(first.result.current.isSuccess).toBe(true);
    });

    const second = renderHook(useSystemInfo, { wrapper: harness.wrapper });

    expect(second.result.current.data).toEqual(fakeSystemInfo);
    expect(second.result.current.isStale).toBe(false);
    expect(harness.requests).toHaveLength(1);
  });

  it("retries once and then reports the ApiError", async () => {
    const harness = createHarness(() => Promise.reject(new TypeError("Failed to fetch")));

    const { result } = renderHook(useSystemInfo, { wrapper: harness.wrapper });

    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
    expect(result.current.error).toBeInstanceOf(ApiError);
    expect((result.current.error as ApiError).code).toBe(clientErrorCodes.networkError);
    expect(harness.requests).toHaveLength(2);
  });
});

import { createApiClient } from "@mivo/api-client";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ApiClientProvider, useApiClient } from "./ApiClientProvider";

describe("useApiClient", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("returns the client of the provider", () => {
    const client = createApiClient({ baseUrl: "http://server.test" });
    const wrapper = ({ children }: { readonly children: ReactNode }) => (
      <ApiClientProvider client={client}>{children}</ApiClientProvider>
    );

    const { result } = renderHook(useApiClient, { wrapper });

    expect(result.current).toBe(client);
  });

  it("fails loudly outside a provider", () => {
    // React logs the error thrown during render; the assertion below is what matters.
    vi.spyOn(console, "error").mockImplementation(() => undefined);

    expect(() => renderHook(useApiClient)).toThrow(
      "useApiClient must be used inside an ApiClientProvider",
    );
  });
});

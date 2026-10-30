import type { ApiClient } from "@mivo/api-client";
import { createContext, useContext, type ReactNode } from "react";

// No default client: a component rendered outside the provider is a wiring bug and must fail
// loudly instead of silently talking to some origin.
const ApiClientContext = createContext<ApiClient | null>(null);

/** Props of {@link ApiClientProvider}. */
export interface ApiClientProviderProps {
  /** The client every API hook below uses; tests pass one with a fake fetch. */
  readonly client: ApiClient;
  readonly children: ReactNode;
}

/** Makes the server API client available to the hooks of every feature. */
export function ApiClientProvider({ client, children }: ApiClientProviderProps) {
  return <ApiClientContext value={client}>{children}</ApiClientContext>;
}

/**
 * Returns the API client of the nearest {@link ApiClientProvider}.
 * @throws Error when no provider is above the calling component.
 */
export function useApiClient(): ApiClient {
  const client = useContext(ApiClientContext);
  if (client === null) {
    throw new Error("useApiClient must be used inside an ApiClientProvider");
  }
  return client;
}

import { ApiError, requestData, type ApiClient } from "@mivo/api-client";
import { queryOptions } from "@tanstack/react-query";

/** Reads the online metadata settings; keys are never part of the answer. */
function fetchMetadataSettings(client: ApiClient, signal: AbortSignal) {
  return requestData(client.GET("/api/v1/settings/metadata", { signal }));
}

/** Reads the queue length and provider states of the online metadata feature. */
function fetchMetadataStatus(client: ApiClient, signal: AbortSignal) {
  return requestData(client.GET("/api/v1/metadata/status", { signal }));
}

// The api-client package exports no names for these schemas yet; deriving them from the
// generated operations keeps this feature in step with the contract without editing it.
/** Online metadata settings as the server reports them. */
export type MetadataSettings = Awaited<ReturnType<typeof fetchMetadataSettings>>;
/** Settings and key status of one provider. */
export type MetadataProviderSetting = MetadataSettings["providers"][number];
/** Queue length and provider states. */
export type MetadataStatus = Awaited<ReturnType<typeof fetchMetadataStatus>>;
/** Current state of one provider. */
export type MetadataProviderStatus = MetadataStatus["providers"][number];

/** Changes to the settings; omitted properties stay as they are. */
export interface MetadataSettingsUpdate {
  readonly onlineEnabled?: boolean;
  readonly language?: string;
  readonly providers?: readonly {
    readonly provider: MetadataProviderSetting["provider"];
    readonly enabled: boolean;
  }[];
  /** Write-only; an empty string removes the own key. */
  readonly tmdbApiKey?: string;
}

/** Cache key of the metadata settings. */
export const metadataSettingsQueryKey = ["settings", "metadata"] as const;

/** Cache key of the metadata status. */
export const metadataStatusQueryKey = ["metadata", "status"] as const;

/** How often the status panel asks again while it is shown. */
export const metadataStatusPollMilliseconds = 5000;

const notImplementedStatus = 501;

/** Whether `error` says the server does not serve online metadata yet (501). */
export function isNotImplemented(error: unknown): boolean {
  return error instanceof ApiError && error.status === notImplementedStatus;
}

// A server without the feature answers 501 every time; retrying only delays the notice.
function retryUnlessNotImplemented(failureCount: number, error: Error): boolean {
  const maxRetries = 2;
  return !isNotImplemented(error) && failureCount < maxRetries;
}

/** Query options for the metadata settings. */
export function metadataSettingsQueryOptions(client: ApiClient) {
  return queryOptions({
    queryKey: metadataSettingsQueryKey,
    queryFn: ({ signal }) => fetchMetadataSettings(client, signal),
    retry: retryUnlessNotImplemented,
  });
}

/** Query options for the metadata status; it polls until the server says it has no such feature. */
export function metadataStatusQueryOptions(client: ApiClient) {
  return queryOptions({
    queryKey: metadataStatusQueryKey,
    queryFn: ({ signal }) => fetchMetadataStatus(client, signal),
    retry: retryUnlessNotImplemented,
    refetchInterval: (query) =>
      isNotImplemented(query.state.error) ? false : metadataStatusPollMilliseconds,
  });
}

/** Changes the metadata settings and resolves with the settings after the change. */
export function updateMetadataSettings(
  client: ApiClient,
  body: MetadataSettingsUpdate,
): Promise<MetadataSettings> {
  // The generated body type has mutable arrays; copying keeps the readonly input untouched.
  const { providers, ...rest } = body;
  return requestData(
    client.PUT("/api/v1/settings/metadata", {
      body:
        providers === undefined
          ? rest
          : { ...rest, providers: providers.map((toggle) => ({ ...toggle })) },
    }),
  );
}

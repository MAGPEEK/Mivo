import { ApiError, requestData, type ApiClient } from "@mivo/api-client";
import { queryOptions } from "@tanstack/react-query";

/** Reads the hardware acceleration setting and the last test result. */
function fetchTranscoderStatus(client: ApiClient, signal: AbortSignal) {
  return requestData(client.GET("/api/v1/system/transcoder", { signal }));
}

/** Lists the running playback sessions of all clients. */
function fetchSessions(client: ApiClient, signal: AbortSignal) {
  return requestData(client.GET("/api/v1/playback/sessions", { signal }));
}

// The api-client package exports no names for these schemas yet; deriving them from the
// generated operations keeps this feature in step with the contract without editing it.
/** Hardware acceleration of the transcoder as the server reports it. */
export type TranscoderStatus = Awaited<ReturnType<typeof fetchTranscoderStatus>>;
/** One tested hardware encoder. */
export type TranscoderDevice = TranscoderStatus["devices"][number];
/** One running playback session. */
export type SessionSummary = Awaited<ReturnType<typeof fetchSessions>>["sessions"][number];

/** Cache key of the transcoder status. */
export const transcoderStatusQueryKey = ["system", "transcoder"] as const;

/** Cache key of the running sessions. */
export const transcoderSessionsQueryKey = ["playback", "sessions"] as const;

/** How often status and session list ask again while the page is visible. */
export const transcoderPollMilliseconds = 5000;

/**
 * How long the page waits for a new test result. A failed detection keeps the old
 * `lastTestedAt`, so without a limit the page would wait for ever.
 */
export const testWatchMilliseconds = 60_000;

/** How often the status is read while a hardware test runs. */
export const testPollMilliseconds = 1000;

const conflictStatus = 409;
const forbiddenStatus = 403;
const maxRetries = 1;

/** Whether `error` says the caller may not manage the server (403). */
export function isForbidden(error: unknown): boolean {
  return (
    error instanceof ApiError && (error.status === forbiddenStatus || error.code === "forbidden")
  );
}

/** Whether `error` says a test run is active already (409). */
export function isTestRunning(error: unknown): boolean {
  return error instanceof ApiError && error.status === conflictStatus;
}

// A refused caller stays refused; retrying only delays the hint.
function retryUnlessForbidden(failureCount: number, error: Error): boolean {
  return !isForbidden(error) && failureCount < maxRetries;
}

/** Query options for the status; `intervalOf` picks the poll interval from the last answer. */
export function transcoderStatusQueryOptions(
  client: ApiClient,
  intervalOf: (status: TranscoderStatus | undefined) => number,
) {
  return queryOptions({
    queryKey: transcoderStatusQueryKey,
    queryFn: ({ signal }) => fetchTranscoderStatus(client, signal),
    retry: retryUnlessForbidden,
    refetchInterval: (query) =>
      isForbidden(query.state.error) ? false : intervalOf(query.state.data),
  });
}

/** Query options for the session list; it polls until the server refuses the caller. */
export function transcoderSessionsQueryOptions(client: ApiClient) {
  return queryOptions({
    queryKey: transcoderSessionsQueryKey,
    queryFn: ({ signal }) => fetchSessions(client, signal),
    retry: retryUnlessForbidden,
    refetchInterval: (query) =>
      isForbidden(query.state.error) ? false : transcoderPollMilliseconds,
  });
}

/** Starts a hardware test run; resolves with the status before its result (202). */
export function startHardwareTest(client: ApiClient): Promise<TranscoderStatus> {
  return requestData(client.POST("/api/v1/system/transcoder/test"));
}

/** Stops a session; idempotent, so an already stopped one also resolves. */
export async function stopSession(client: ApiClient, sessionId: string): Promise<void> {
  await requestData(
    client.DELETE("/api/v1/playback/sessions/{sessionId}", { params: { path: { sessionId } } }),
  );
}

import type { ApiClient, ProgressReport } from "@mivo/api-client";

const progressPath = "/api/v1/items/{itemId}/progress";

const unauthenticatedStatus = 401;
const forbiddenStatus = 403;
const notImplementedStatus = 501;

/**
 * Answers a later report would get again: no session or profile (the app's sign-in handling
 * reacts to those), and a server that does not store watch state yet.
 */
const acceptedFailureStatuses: readonly number[] = [
  unauthenticatedStatus,
  forbiddenStatus,
  notImplementedStatus,
];

/** The server answered a progress report with an error worth another try. */
export class ProgressReportError extends Error {
  readonly status: number;

  constructor(status: number) {
    super(`progress report failed with status ${String(status)}`);
    this.name = "ProgressReportError";
    this.status = status;
  }
}

/** Sends one position report of the item a sender was created for. */
export type SendProgressReport = (report: ProgressReport) => Promise<void>;

/**
 * Reports the playback position of `itemId` (`PUT /api/v1/items/{itemId}/progress`, ADR 0032
 * section 8) through the generated API client. `keepalive` lets the report outlive the page,
 * because the last one is sent on `pagehide` and on leaving the player.
 *
 * The promise resolves when the position is stored and also for 401, 403 and 501, which the
 * player must not notice. It rejects for a network failure or any other error status, so the
 * reporter sends the position again with its next report; nothing is shown to the user.
 */
export function createProgressSender(client: ApiClient, itemId: string): SendProgressReport {
  return async (report) => {
    const { response } = await client.PUT(progressPath, {
      params: { path: { itemId } },
      body: report,
      keepalive: true,
    });
    if (!response.ok && !acceptedFailureStatuses.includes(response.status)) {
      throw new ProgressReportError(response.status);
    }
  };
}

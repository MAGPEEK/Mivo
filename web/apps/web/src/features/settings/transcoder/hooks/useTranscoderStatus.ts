import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { useApiClient } from "../../../../shared/api";
import {
  isForbidden,
  isTestRunning,
  startHardwareTest,
  testPollMilliseconds,
  transcoderPollMilliseconds,
  transcoderStatusQueryKey,
  transcoderStatusQueryOptions,
  type TranscoderStatus,
} from "../api/transcoderApi";

/** Why starting the hardware test failed; null while nothing failed. */
export type TestFailure = "forbidden" | "failed" | null;

/** State and action of the "Test hardware acceleration" button. */
export interface HardwareTest {
  /** True from the click until the status carries a new `lastTestedAt`. */
  readonly running: boolean;
  /** True once a run that this page watched has finished. */
  readonly finished: boolean;
  readonly failure: TestFailure;
  readonly start: () => void;
}

/** What the status panel shows. */
export type TranscoderLoad =
  | { readonly state: "loading" }
  | { readonly state: "forbidden" }
  | { readonly state: "error"; readonly retry: () => void }
  | { readonly state: "ready"; readonly status: TranscoderStatus; readonly test: HardwareTest };

// The server reports a finished run only by a new lastTestedAt, so the value before the run is
// what later statuses are compared with. `undefined` inside means "never tested before".
interface Baseline {
  readonly lastTestedAt: string | undefined;
}

function isWatchedRunActive(baseline: Baseline | null, status: TranscoderStatus | undefined) {
  return baseline !== null && status !== undefined && status.lastTestedAt === baseline.lastTestedAt;
}

function failureOf(error: Error | null): TestFailure {
  if (error === null || isTestRunning(error)) {
    return null;
  }
  return isForbidden(error) ? "forbidden" : "failed";
}

function useHardwareTest(
  status: TranscoderStatus | undefined,
  baseline: Baseline | null,
  watch: (baseline: Baseline) => void,
): HardwareTest {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: () => startHardwareTest(client),
    onSuccess: (before) => {
      queryClient.setQueryData(transcoderStatusQueryKey, before);
      watch({ lastTestedAt: before.lastTestedAt });
    },
    onError: (error: Error) => {
      // A run is active already (409): wait for that one instead of reporting a failure.
      if (isTestRunning(error) && status !== undefined) {
        watch({ lastTestedAt: status.lastTestedAt });
      }
    },
  });
  const running = mutation.isPending || isWatchedRunActive(baseline, status);
  const failure = failureOf(mutation.error);
  return {
    running,
    finished: baseline !== null && !running && failure === null,
    failure,
    start: () => {
      mutation.mutate();
    },
  };
}

/**
 * Hardware acceleration status, polled while the page is visible (faster while a test runs),
 * and the test action. A 403 stops polling and reports `forbidden`.
 */
export function useTranscoderStatus(): TranscoderLoad {
  const client = useApiClient();
  const [baseline, setBaseline] = useState<Baseline | null>(null);
  const query = useQuery(
    transcoderStatusQueryOptions(client, (status) =>
      isWatchedRunActive(baseline, status) ? testPollMilliseconds : transcoderPollMilliseconds,
    ),
  );
  const test = useHardwareTest(query.data, baseline, setBaseline);
  if (query.data !== undefined) {
    return { state: "ready", status: query.data, test };
  }
  if (query.error === null) {
    return { state: "loading" };
  }
  return isForbidden(query.error)
    ? { state: "forbidden" }
    : { state: "error", retry: () => void query.refetch() };
}

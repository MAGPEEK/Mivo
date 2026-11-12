import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import { useApiClient } from "../../../../shared/api";
import {
  isForbidden,
  isTestRunning,
  startHardwareTest,
  testPollMilliseconds,
  testWatchMilliseconds,
  transcoderPollMilliseconds,
  transcoderStatusQueryKey,
  transcoderStatusQueryOptions,
  type TranscoderStatus,
} from "../api/transcoderApi";

/** Why starting the hardware test failed; null while nothing failed. */
export type TestFailure = "forbidden" | "failed" | null;

/** State and action of the "Test hardware acceleration" button. */
export interface HardwareTest {
  /** True from the click until the status carries a new `lastTestedAt` or the watch expires. */
  readonly running: boolean;
  /** True once a run that this page watched has finished with a new result. */
  readonly finished: boolean;
  /** True when the watch expired without a new result, for example because detection failed. */
  readonly stalled: boolean;
  /** True while the page waits for a run that was active already (409). */
  readonly alreadyRunning: boolean;
  readonly failure: TestFailure;
  readonly start: () => void;
}

/** What the status panel shows. */
export type TranscoderLoad =
  | { readonly state: "loading" }
  | { readonly state: "forbidden" }
  | { readonly state: "error"; readonly retry: () => void }
  | { readonly state: "ready"; readonly status: TranscoderStatus; readonly test: HardwareTest };

// The server reports a finished run only by a new lastTestedAt, and keeps the old one when
// detection fails; so the watch ends by a new value or by a time limit (`expired`).
interface Watch {
  readonly lastTestedAt: string | undefined;
  readonly expired: boolean;
}

function isUnchanged(watch: Watch | null, status: TranscoderStatus | undefined): boolean {
  return watch !== null && status !== undefined && status.lastTestedAt === watch.lastTestedAt;
}

function isWatchedRunActive(watch: Watch | null, status: TranscoderStatus | undefined): boolean {
  return watch !== null && !watch.expired && isUnchanged(watch, status);
}

function failureOf(error: Error | null): TestFailure {
  if (error === null || isTestRunning(error)) {
    return null;
  }
  return isForbidden(error) ? "forbidden" : "failed";
}

function useWatchExpiry(watch: Watch | null, setWatch: (watch: Watch) => void) {
  useEffect(() => {
    if (watch === null || watch.expired) {
      return undefined;
    }
    const timer = setTimeout(() => {
      setWatch({ ...watch, expired: true });
    }, testWatchMilliseconds);
    return () => {
      clearTimeout(timer);
    };
  }, [watch, setWatch]);
}

function useHardwareTest(
  status: TranscoderStatus | undefined,
  watch: Watch | null,
  setWatch: (watch: Watch) => void,
): HardwareTest {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: () => startHardwareTest(client),
    onSuccess: (before) => {
      queryClient.setQueryData(transcoderStatusQueryKey, before);
      setWatch({ lastTestedAt: before.lastTestedAt, expired: false });
    },
    onError: (error: Error) => {
      // A run is active already (409): wait for that one instead of reporting a failure.
      if (isTestRunning(error) && status !== undefined) {
        setWatch({ lastTestedAt: status.lastTestedAt, expired: false });
      }
    },
  });
  const running = mutation.isPending || isWatchedRunActive(watch, status);
  const failure = failureOf(mutation.error);
  const stalled = watch !== null && watch.expired && isUnchanged(watch, status);
  return {
    running,
    finished: watch !== null && !running && !stalled && failure === null,
    stalled,
    alreadyRunning: running && isTestRunning(mutation.error),
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
  const [watch, setWatch] = useState<Watch | null>(null);
  useWatchExpiry(watch, setWatch);
  const query = useQuery(
    transcoderStatusQueryOptions(client, (status) =>
      isWatchedRunActive(watch, status) ? testPollMilliseconds : transcoderPollMilliseconds,
    ),
  );
  const test = useHardwareTest(query.data, watch, setWatch);
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

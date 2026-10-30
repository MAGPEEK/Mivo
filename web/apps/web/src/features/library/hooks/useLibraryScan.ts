import type { Library, ScanStatus } from "@mivo/api-client";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useRef } from "react";

import { useApiClient } from "../../../shared/api";
import {
  isScanRunning,
  librariesQueryKey,
  libraryQueryKey,
  startLibraryScan,
  withScanStatus,
} from "../api/libraryQueries";

/** Scan state of a library and the action that starts a scan. */
export interface LibraryScan {
  /**
   * True from the click until the scan has finished. The scan button is `aria-disabled`
   * meanwhile (not `disabled`, which would drop keyboard focus) and `start` does nothing.
   */
  readonly running: boolean;
  /** Starts a scan; ignored while one runs or is being started. */
  readonly start: () => void;
  /**
   * Why the last start request failed, or null. Null as well once a scan runs or the library
   * reports another scan than at the click (e.g. one started at server start): the error is
   * stale then. A failed *scan* is in the library status.
   */
  readonly startError: Error | null;
}

/** What the scan button knew when it was clicked. */
interface StartRequest {
  /** `startedAt` of the last scan at the click; undefined before the first scan. */
  readonly startedAtBefore: string | undefined;
}

/**
 * Starts scans of `library` and refreshes everything a scan changes once it ends. The
 * started status goes straight into the library cache, so polling (see `useLibrary`) begins
 * without waiting for another request.
 */
export function useLibraryScan(library: Library): LibraryScan {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const libraryId = library.id;
  // The request is kept as the mutation's variables for the staleness check below; the
  // server call itself needs nothing from it.
  const mutation = useMutation<ScanStatus, Error, StartRequest>({
    mutationFn: () => startLibraryScan(client, libraryId),
    onSuccess: async (scan: ScanStatus) => {
      const libraryKey = libraryQueryKey(libraryId);
      // A poll still in flight would overwrite the new status with the one before the start.
      // Exact: the item pages share this key prefix, and a "Load more" must not be dropped.
      await queryClient.cancelQueries({ queryKey: libraryKey, exact: true });
      queryClient.setQueryData<Library>(libraryKey, (current) => withScanStatus(current, scan));
      // A tiny library can be done before the response is sent; no running phase to watch.
      if (!isScanRunning(scan)) {
        await queryClient.invalidateQueries({ queryKey: librariesQueryKey });
      }
    },
  });
  const scanRunning = isScanRunning(library.scan);
  useRefreshAfterScan(scanRunning);
  const running = scanRunning || mutation.isPending;
  const { mutate } = mutation;
  const startedAt = library.scan.startedAt;
  const start = useCallback(() => {
    if (!running) {
      mutate({ startedAtBefore: startedAt });
    }
  }, [mutate, running, startedAt]);
  // Compares server timestamps only: the client clock may differ from the server clock.
  const anotherScanSeen =
    mutation.variables !== undefined && mutation.variables.startedAtBefore !== startedAt;
  const staleError = scanRunning || anotherScanSeen;
  return {
    running,
    start,
    startError: staleError ? null : mutation.error,
  };
}

// When a scan ends, items, counts and availability have changed: refresh every library query
// (the sidebar list, this library and its item pages). Also covers scans started elsewhere,
// e.g. at server start, as long as the page saw them running.
function useRefreshAfterScan(scanRunning: boolean): void {
  const queryClient = useQueryClient();
  const wasRunning = useRef(scanRunning);
  useEffect(() => {
    if (wasRunning.current && !scanRunning) {
      void queryClient.invalidateQueries({ queryKey: librariesQueryKey });
    }
    wasRunning.current = scanRunning;
  }, [queryClient, scanRunning]);
}

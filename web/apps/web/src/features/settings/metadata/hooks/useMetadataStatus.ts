import { useQuery } from "@tanstack/react-query";

import { useApiClient } from "../../../../shared/api";
import {
  isNotImplemented,
  metadataStatusQueryOptions,
  type MetadataStatus,
} from "../api/metadataSettings";

/** What the status panel shows. */
export type MetadataStatusLoad =
  | { readonly status: "loading" }
  /** The server does not serve online metadata yet (501); polling has stopped. */
  | { readonly status: "unavailable" }
  | { readonly status: "error" }
  | { readonly status: "ready"; readonly metadataStatus: MetadataStatus };

/**
 * Queue length and provider states, polled while the caller is mounted. A failed poll keeps
 * the last answer on screen; the next poll may succeed again.
 */
export function useMetadataStatus(): MetadataStatusLoad {
  const client = useApiClient();
  const query = useQuery(metadataStatusQueryOptions(client));
  if (query.data !== undefined) {
    return { status: "ready", metadataStatus: query.data };
  }
  if (query.error === null) {
    return { status: "loading" };
  }
  return isNotImplemented(query.error) ? { status: "unavailable" } : { status: "error" };
}

import type { IdentifyCandidate, IdentifySearchRequest, ItemDetail } from "@mivo/api-client";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useCallback, useState } from "react";

import { useApiClient } from "../../../shared/api";
import { identifyItem, identifySearchKey, searchCandidates } from "../api/identifyApi";
import { identifyErrorKind, type IdentifyErrorKind } from "../lib/identifyError";
import { isSameSearch, toSearchRequest } from "../lib/searchInput";

/** Options of {@link useIdentify}. */
export interface UseIdentifyOptions {
  readonly itemId: string;
  /** Title of the item; prefills the search. */
  readonly title: string;
  /** Year of the item such as `2008`; prefills the search, undefined when unknown. */
  readonly year: string | undefined;
  /** Called with the matched item once the server accepted the match. */
  readonly onApplied: (item: ItemDetail) => void;
  /** Closes the dialog. */
  readonly onClose: () => void;
}

/** The result list of the identify dialog. */
export type IdentifyResults =
  | { readonly status: "idle" | "pending" }
  | { readonly status: "success"; readonly candidates: readonly IdentifyCandidate[] }
  | { readonly status: "error"; readonly errorKind: IdentifyErrorKind };

/** State and actions of the identify dialog. */
export interface IdentifyState {
  readonly query: string;
  readonly year: string;
  readonly setQuery: (query: string) => void;
  readonly setYear: (year: string) => void;
  /** Searches with the current fields; repeats the search when they did not change. */
  readonly submit: () => void;
  readonly results: IdentifyResults;
  /** Matches the item with a candidate; ignored while a match is being applied. */
  readonly apply: (candidate: IdentifyCandidate) => void;
  /** Key ({@link candidateKey}) of the candidate being applied, else undefined. */
  readonly applyingKey: string | undefined;
  /** Why the last apply failed; undefined when it did not. */
  readonly applyErrorKind: IdentifyErrorKind | undefined;
  /**
   * Closes the dialog; ignored while a match is being applied, because the server would still
   * apply it after the dialog is gone.
   */
  readonly close: () => void;
}

/** Stable key of a candidate within a result list. */
export function candidateKey(candidate: IdentifyCandidate): string {
  return `${candidate.provider}:${candidate.externalId}`;
}

/**
 * Logic of the identify dialog: the search fields prefilled with title and year, the search
 * (run at once with the prefill and again on every submit) and applying a candidate.
 */
export function useIdentify(options: UseIdentifyOptions): IdentifyState {
  const { itemId, title } = options;
  const [query, setQuery] = useState(title);
  const [year, setYear] = useState(options.year ?? "");
  const [submitted, setSubmitted] = useState(() =>
    toSearchRequest({ query: title, year: options.year ?? "", itemTitle: title }),
  );
  const results = useCandidateSearch(itemId, submitted);
  const { refetch } = results;
  const { reset: resetApply, ...applying } = useApplyMatch(itemId, options.onApplied);
  const submit = () => {
    // A new search makes an apply error of the old result list meaningless.
    resetApply();
    const request = toSearchRequest({ query, year, itemTitle: title });
    if (isSameSearch(request, submitted)) {
      void refetch();
    }
    setSubmitted(request);
  };
  const { onClose } = options;
  const blocked = applying.applyingKey !== undefined;
  const close = useCallback(() => {
    if (!blocked) {
      onClose();
    }
  }, [blocked, onClose]);
  return { query, year, setQuery, setYear, submit, results: results.state, close, ...applying };
}

function useCandidateSearch(itemId: string, request: IdentifySearchRequest | undefined) {
  const client = useApiClient();
  const query = useQuery({
    queryKey: identifySearchKey(itemId, request ?? { query: "" }),
    queryFn: ({ signal }) => {
      // `enabled` keeps the query from running without a request.
      if (request === undefined) {
        return [];
      }
      return searchCandidates(client, itemId, request, signal);
    },
    enabled: request !== undefined,
    // Provider errors are answered by the dialog's retry; candidates go stale with the dialog.
    retry: false,
    gcTime: 0,
  });
  return { state: toResults(request, query), refetch: query.refetch };
}

interface SearchQueryState {
  readonly data: readonly IdentifyCandidate[] | undefined;
  readonly error: Error | null;
  readonly isFetching: boolean;
}

function toResults(
  request: IdentifySearchRequest | undefined,
  query: SearchQueryState,
): IdentifyResults {
  if (request === undefined) {
    return { status: "idle" };
  }
  if (query.isFetching) {
    return { status: "pending" };
  }
  if (query.error !== null) {
    return { status: "error", errorKind: identifyErrorKind(query.error) };
  }
  return query.data === undefined
    ? { status: "pending" }
    : { status: "success", candidates: query.data };
}

function useApplyMatch(itemId: string, onApplied: (item: ItemDetail) => void) {
  const client = useApiClient();
  const mutation = useMutation({
    mutationFn: (candidate: IdentifyCandidate) =>
      identifyItem(client, itemId, {
        provider: candidate.provider,
        externalId: candidate.externalId,
      }),
    onSuccess: onApplied,
  });
  const { mutate, isPending } = mutation;
  const apply = useCallback(
    (candidate: IdentifyCandidate) => {
      if (!isPending) {
        mutate(candidate);
      }
    },
    [mutate, isPending],
  );
  return {
    apply,
    applyingKey: isPending ? candidateKey(mutation.variables) : undefined,
    applyErrorKind: mutation.error === null ? undefined : identifyErrorKind(mutation.error),
    reset: mutation.reset,
  };
}

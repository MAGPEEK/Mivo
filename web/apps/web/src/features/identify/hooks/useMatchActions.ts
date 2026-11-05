import type { ItemDetail } from "@mivo/api-client";
import { useMutation } from "@tanstack/react-query";
import { useCallback, useState } from "react";

import { useApiClient } from "../../../shared/api";
import { refreshItemMetadata, removeItemMatch } from "../api/identifyApi";
import { identifyErrorKind, type IdentifyErrorKind } from "../lib/identifyError";

/** Which dialog of the match actions is open. */
export type MatchDialog = "none" | "identify" | "remove";

/** The outcome of the last match action, announced next to the menu. */
export type MatchOutcome =
  | { readonly kind: "applied" | "removed" | "refreshQueued" }
  | { readonly kind: "refreshFailed"; readonly errorKind: IdentifyErrorKind };

/** Options of {@link useMatchActions}. */
export interface UseMatchActionsOptions {
  readonly itemId: string;
  /** Called after the item changed on the server, with its new state when the server sent it. */
  readonly onItemChanged: (item?: ItemDetail) => void;
}

/** State and actions behind the "Correct match", "Remove match" and "Refresh" menu entries. */
export interface MatchActions {
  readonly dialog: MatchDialog;
  readonly openIdentify: () => void;
  readonly openRemove: () => void;
  readonly closeDialog: () => void;
  /** Called by the identify dialog once a match was applied. */
  readonly applied: (item: ItemDetail) => void;
  readonly confirmRemove: () => void;
  readonly removePending: boolean;
  /** Why removing failed; undefined when it did not. */
  readonly removeErrorKind: IdentifyErrorKind | undefined;
  readonly refresh: () => void;
  readonly refreshPending: boolean;
  /** Undefined until an action finished. */
  readonly outcome: MatchOutcome | undefined;
}

type Finish = (outcome: MatchOutcome, item?: ItemDetail) => void;

/** Logic of the match actions of a detail page. */
export function useMatchActions({ itemId, onItemChanged }: UseMatchActionsOptions): MatchActions {
  const [dialog, setDialog] = useState<MatchDialog>("none");
  const [outcome, setOutcome] = useState<MatchOutcome | undefined>(undefined);
  const finish = useCallback<Finish>(
    (next, item) => {
      setDialog("none");
      setOutcome(next);
      if (next.kind !== "refreshFailed") {
        onItemChanged(item);
      }
    },
    [onItemChanged],
  );
  const remove = useRemoveMatch(itemId, finish);
  const refresh = useRefresh(itemId, finish, () => {
    setOutcome(undefined);
  });
  const { reset: resetRemove } = remove;
  const open = useCallback(
    (next: MatchDialog) => {
      resetRemove();
      setOutcome(undefined);
      setDialog(next);
    },
    [resetRemove],
  );
  return {
    dialog,
    openIdentify: () => {
      open("identify");
    },
    openRemove: () => {
      open("remove");
    },
    closeDialog: useCallback(() => {
      setDialog("none");
    }, []),
    applied: (item: ItemDetail) => {
      finish({ kind: "applied" }, item);
    },
    confirmRemove: remove.confirm,
    removePending: remove.pending,
    removeErrorKind: remove.errorKind,
    refresh: refresh.start,
    refreshPending: refresh.pending,
    outcome,
  };
}

function useRemoveMatch(itemId: string, finish: Finish) {
  const client = useApiClient();
  const mutation = useMutation({
    mutationFn: () => removeItemMatch(client, itemId),
    onSuccess: (item) => {
      finish({ kind: "removed" }, item);
    },
  });
  const { mutate, isPending } = mutation;
  return {
    confirm: useCallback(() => {
      if (!isPending) {
        mutate();
      }
    }, [mutate, isPending]),
    pending: isPending,
    errorKind: mutation.error === null ? undefined : identifyErrorKind(mutation.error),
    reset: mutation.reset,
  };
}

function useRefresh(itemId: string, finish: Finish, clearOutcome: () => void) {
  const client = useApiClient();
  const mutation = useMutation({
    mutationFn: () => refreshItemMetadata(client, itemId),
    onSuccess: () => {
      finish({ kind: "refreshQueued" });
    },
    onError: (error) => {
      finish({ kind: "refreshFailed", errorKind: identifyErrorKind(error) });
    },
  });
  const { mutate, isPending } = mutation;
  return {
    start: () => {
      if (!isPending) {
        clearOutcome();
        mutate();
      }
    },
    pending: isPending,
  };
}

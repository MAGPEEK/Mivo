import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";

import { isItemNotFound, isNetworkFailure } from "../api/itemQuery";
import type { DurationParts } from "../lib/duration";
import { toItemView, type ItemView, type ItemViewTexts } from "../lib/itemView";
import type { UnavailableReason } from "../lib/playTarget";
import { itemNamespace } from "../locales";
import { useItem } from "./useItem";

type ItemTranslate = TFunction<typeof itemNamespace>;

/** Why loading failed: the server was unreachable, or it answered with an error. */
export type LoadFailure = "network" | "server";

/**
 * Changes whenever a load of the item settles, also when an error repeats or React skips the
 * intermediate loading render; lets the page react to the end of a retry.
 */
type Settlement = string;

/** State of the detail page. */
export type ItemDetailsState =
  | { readonly status: "loading" }
  | { readonly status: "notFound" }
  | {
      readonly status: "error";
      readonly failure: LoadFailure;
      readonly retry: () => void;
      readonly settlement: Settlement;
    }
  | { readonly status: "ready"; readonly item: ItemView; readonly settlement: Settlement };

/** Loads an item and turns it into the texts and chips of the detail page. */
export function useItemDetails(itemId: string): ItemDetailsState {
  const { t, i18n } = useTranslation(itemNamespace);
  const query = useItem(itemId);
  const settlement = `${String(query.dataUpdatedAt)}:${String(query.errorUpdateCount)}`;
  // Loaded data wins over a failed background refetch: the page keeps what it showed.
  if (query.data !== undefined) {
    const item = toItemView(query.data, viewTexts(t, i18n.language));
    return { status: "ready", item, settlement };
  }
  if (!query.isError) {
    return { status: "loading" };
  }
  if (isItemNotFound(query.error)) {
    return { status: "notFound" };
  }
  // A refetch of a query without data goes back to pending, so a retry shows "loading".
  return {
    status: "error",
    failure: isNetworkFailure(query.error) ? "network" : "server",
    retry: () => void query.refetch(),
    settlement,
  };
}

function viewTexts(t: ItemTranslate, language: string): ItemViewTexts {
  return {
    language,
    duration: (parts) => durationText(parts, t),
    unknownLanguage: t("tracks.unknownLanguage"),
    unavailable: (reason) => unavailableExplanation(reason, t),
  };
}

function durationText(parts: DurationParts, t: ItemTranslate): string {
  switch (parts.unit) {
    case "hours":
      return t("duration.hoursMinutes", { hours: parts.hours, minutes: parts.minutes });
    case "minutes":
      return t("duration.minutes", { minutes: parts.minutes });
    case "seconds":
      return t("duration.seconds", { seconds: parts.seconds });
  }
}

function unavailableExplanation(reason: UnavailableReason, t: ItemTranslate): string {
  switch (reason) {
    case "noFile":
      return t("play.unavailable.noFile");
    case "probeFailed":
      return t("play.unavailable.probeFailed");
    case "probeTimedOut":
      return t("play.unavailable.probeTimedOut");
    case "unknown":
      return t("play.unavailable.generic");
  }
}

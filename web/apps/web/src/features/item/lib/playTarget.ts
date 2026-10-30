import type { ItemDetail } from "@mivo/api-client";

/** Why an item cannot be played; `unknown` covers error codes of later server versions. */
export type UnavailableReason = "noFile" | "probeFailed" | "probeTimedOut" | "unknown";

/** What the Play button does: open the player for a file, or explain why it cannot. */
export type PlayTarget =
  | { readonly kind: "playable"; readonly fileId: string }
  | { readonly kind: "unavailable"; readonly reason: UnavailableReason };

// Codes the server stores when probing a file fails (`internal/domain/media`, ProbeError).
const reasonsByErrorCode: ReadonlyMap<string, UnavailableReason> = new Map([
  ["probe_failed", "probeFailed"],
  ["probe_timed_out", "probeTimedOut"],
]);

/** The Play target of an item: its primary (first) file, when that file can be played. */
export function playTarget(item: ItemDetail): PlayTarget {
  const [primaryFile] = item.files;
  if (primaryFile === undefined) {
    return { kind: "unavailable", reason: "noFile" };
  }
  if (primaryFile.available) {
    return { kind: "playable", fileId: primaryFile.id };
  }
  const reason = reasonsByErrorCode.get(primaryFile.errorCode ?? "") ?? "unknown";
  return { kind: "unavailable", reason };
}

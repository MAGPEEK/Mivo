import type { IdentifyCandidate } from "@mivo/api-client";

// The contract promises server URLs only (ADR 0010); anything else is dropped, so the browser
// never contacts a provider host even if a server or proxy sent one.
const serverImagePrefix = "/api/v1/";

/** The poster URL of a candidate when it is a server URL, else undefined. */
export function candidatePosterSrc(candidate: IdentifyCandidate): string | undefined {
  const url = candidate.posterUrl;
  return url?.startsWith(serverImagePrefix) === true ? url : undefined;
}

/** Title of a candidate with its year, e.g. `Big Buck Bunny (2008)`, for accessible names. */
export function candidateTitle(candidate: IdentifyCandidate): string {
  return candidate.year === undefined
    ? candidate.title
    : `${candidate.title} (${String(candidate.year)})`;
}

import type { IdentifySearchRequest, ItemKind, MetadataProvider } from "@mivo/api-client";

// Bounds of IdentifySearchRequest in api/openapi.yaml.
const maxQueryLength = 256;
const maxProviderIdLength = 512;
const minYear = 1800;
const maxYear = 2200;

// A link, a prefixed TMDB ID ("tmdb:10378") or a MusicBrainz ID (a UUID). A bare number is not
// a reference: titles such as "1917" or "300" would be looked up as IDs.
const urlPattern = /^https?:\/\/\S+$/i;
const prefixedTmdbPattern = /^tmdb[:-]\d+$/i;
const musicBrainzIdPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/**
 * Whether the text is a pasted provider link or ID rather than a title, so the server looks up
 * that entry instead of searching.
 */
export function isProviderReference(text: string): boolean {
  const trimmed = text.trim();
  return (
    urlPattern.test(trimmed) ||
    prefixedTmdbPattern.test(trimmed) ||
    musicBrainzIdPattern.test(trimmed)
  );
}

/** The year as the search request takes it; undefined when empty or out of range. */
export function parseSearchYear(text: string): number | undefined {
  const trimmed = text.trim();
  if (!/^\d{4}$/.test(trimmed)) {
    return undefined;
  }
  const year = Number(trimmed);
  return year >= minYear && year <= maxYear ? year : undefined;
}

/** What the user typed into the identify dialog. */
export interface SearchInput {
  readonly query: string;
  readonly year: string;
  /** Title of the item; the query of a lookup by pasted reference, which needs one. */
  readonly itemTitle: string;
}

/**
 * Builds the search request from the dialog fields, or undefined when there is nothing to
 * search for. A pasted reference is sent as `providerId` with the item title as query.
 */
export function toSearchRequest(input: SearchInput): IdentifySearchRequest | undefined {
  const query = input.query.trim();
  if (query === "") {
    return undefined;
  }
  if (isProviderReference(query)) {
    const title = input.itemTitle.trim().slice(0, maxQueryLength);
    return {
      query: title === "" ? query.slice(0, maxQueryLength) : title,
      providerId: query.slice(0, maxProviderIdLength),
    };
  }
  const year = parseSearchYear(input.year);
  return {
    query: query.slice(0, maxQueryLength),
    ...(year === undefined ? {} : { year }),
  };
}

/** Whether two search requests ask the same; false when either is missing. */
export function isSameSearch(
  first: IdentifySearchRequest | undefined,
  second: IdentifySearchRequest | undefined,
): boolean {
  if (first === undefined || second === undefined) {
    return false;
  }
  return (
    first.query === second.query &&
    first.year === second.year &&
    first.providerId === second.providerId
  );
}

// Kinds with an entry of their own at a provider: TMDB movies and shows, MusicBrainz albums
// (release groups) and artists (M6-09). Kinds of later API versions offer no match actions.
const matchableKinds: ReadonlySet<ItemKind> = new Set<ItemKind>([
  "movie",
  "show",
  "album",
  "artist",
]);

/**
 * Whether items of this kind are matched on their own. Seasons and episodes follow their show,
 * tracks their album and extras their parent, so they offer no match actions.
 */
export function isMatchableKind(kind: ItemKind): boolean {
  return matchableKinds.has(kind);
}

// Display names of the providers; they are proper names and not translated.
const providerNames: Readonly<Record<MetadataProvider, string>> = {
  tmdb: "TMDB",
  musicbrainz: "MusicBrainz",
  coverartarchive: "Cover Art Archive",
};

/** Display name of a provider; the raw value for providers this client does not know. */
export function providerName(provider: string): string {
  return Object.hasOwn(providerNames, provider)
    ? providerNames[provider as MetadataProvider]
    : provider;
}

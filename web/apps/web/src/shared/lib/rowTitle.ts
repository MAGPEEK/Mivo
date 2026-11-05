import type { Row } from "@mivo/api-client";

/** The parts of a row its title is built from. */
export type RowTitleSource = Pick<Row, "kind" | "titleKey" | "titleParams" | "genre">;

/** Translation key and values of a row title in the shared namespace. */
export type RowTitleMessage =
  | {
      readonly key: "rows.recentlyAdded" | "rows.newReleases" | "rows.recentEpisodes";
    }
  | { readonly key: "rows.fallback" }
  | { readonly key: "rows.recentlyAddedIn"; readonly values: { readonly library: string } }
  | { readonly key: "rows.moreGenre"; readonly values: { readonly genre: string } };

const plainTitleKeys: Readonly<Record<string, RowTitleMessage>> = {
  "rows.recentlyAdded": { key: "rows.recentlyAdded" },
  "rows.newReleases": { key: "rows.newReleases" },
  "rows.recentEpisodes": { key: "rows.recentEpisodes" },
};

// Titles by kind for a key this client has no translation for (a newer server) or a key whose
// parameter is missing.
const kindTitleKeys: Readonly<Record<string, RowTitleMessage>> = {
  recentlyAdded: { key: "rows.recentlyAdded" },
  newReleases: { key: "rows.newReleases" },
  recentEpisodes: { key: "rows.recentEpisodes" },
};

function nonEmpty(value: string | undefined): string | undefined {
  return value === undefined || value.trim() === "" ? undefined : value;
}

function moreGenreMessage(genre: string | undefined): RowTitleMessage | undefined {
  return genre === undefined ? undefined : { key: "rows.moreGenre", values: { genre } };
}

function fromTitleKey(row: RowTitleSource): RowTitleMessage | undefined {
  const params = row.titleParams ?? {};
  switch (row.titleKey) {
    case "rows.recentlyAddedIn": {
      const library = nonEmpty(params["library"]);
      return library === undefined ? undefined : { key: row.titleKey, values: { library } };
    }
    case "rows.moreGenre":
      return moreGenreMessage(nonEmpty(params["genre"]) ?? nonEmpty(row.genre));
    default:
      return plainTitleKeys[row.titleKey];
  }
}

function fromKind(row: RowTitleSource): RowTitleMessage {
  if (row.kind === "moreGenre") {
    return moreGenreMessage(nonEmpty(row.genre)) ?? { key: "rows.fallback" };
  }
  return kindTitleKeys[row.kind] ?? { key: "rows.fallback" };
}

/**
 * The translated title of a row: the server's key with its parameters when this client knows
 * the key and every parameter is there, else a title for the row's kind, else a generic one.
 * The server sends keys rather than text because it has no UI language.
 */
export function rowTitleMessage(row: RowTitleSource): RowTitleMessage {
  return fromTitleKey(row) ?? fromKind(row);
}

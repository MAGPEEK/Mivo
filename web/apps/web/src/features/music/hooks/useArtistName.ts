import { useQuery } from "@tanstack/react-query";

import { useApiClient } from "../../../shared/api";
import { artistNameQueryOptions } from "../api/musicQueries";

/**
 * The name of the artist `artistId`; `undefined` while it loads, when it fails or without an
 * artist. A card then simply shows no artist line.
 */
export function useArtistName(artistId: string | undefined): string | undefined {
  const client = useApiClient();
  const query = useQuery({
    ...artistNameQueryOptions(client, artistId ?? ""),
    enabled: artistId !== undefined,
  });
  return query.data;
}

import type {
  MetadataProviderSetting,
  MetadataSettings,
  MetadataSettingsUpdate,
} from "../api/metadataSettings";

/** Switches the page shows; MusicBrainz and the Cover Art Archive share one (ADR 0031). */
export type ProviderGroupId = "tmdb" | "music";

type Provider = MetadataProviderSetting["provider"];

const groupMembers: Readonly<Record<ProviderGroupId, readonly Provider[]>> = {
  tmdb: ["tmdb"],
  music: ["musicbrainz", "coverartarchive"],
};

/** One switch: the providers it controls and whether they are on. */
export interface ProviderGroup {
  readonly id: ProviderGroupId;
  /** The members the server reported, in display order. */
  readonly providers: readonly Provider[];
  /** On when every reported member is on. */
  readonly enabled: boolean;
}

/**
 * The switches for `settings`, in display order. A group appears only when the server reports
 * at least one of its members; providers this client does not know are left out.
 */
export function providerGroups(settings: MetadataSettings): ProviderGroup[] {
  const groupIds: readonly ProviderGroupId[] = ["tmdb", "music"];
  return groupIds.flatMap((id) => {
    const members = settings.providers.filter((setting) =>
      groupMembers[id].includes(setting.provider),
    );
    if (members.length === 0) {
      return [];
    }
    return [
      {
        id,
        providers: members.map((setting) => setting.provider),
        enabled: members.every((setting) => setting.enabled),
      },
    ];
  });
}

/** The change that switches every member of `group` to `enabled`. */
export function toggleGroup(group: ProviderGroup, enabled: boolean): MetadataSettingsUpdate {
  return { providers: group.providers.map((provider) => ({ provider, enabled })) };
}

/** Which TMDB key the server uses; unknown or missing values count as none. */
export type TmdbKeyStatus = "bundled" | "own" | "none";

/** The key status of TMDB in `settings`. */
export function tmdbKeyStatus(settings: MetadataSettings): TmdbKeyStatus {
  const status = settings.providers.find((setting) => setting.provider === "tmdb")?.keyStatus;
  return status === "bundled" || status === "own" ? status : "none";
}

/** Languages to offer: the known list plus the stored one when the list lacks it. */
export function languageChoices(known: readonly string[], current: string): readonly string[] {
  return known.includes(current) ? known : [...known, current];
}

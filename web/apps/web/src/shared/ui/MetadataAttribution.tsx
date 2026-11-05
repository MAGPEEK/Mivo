// Emitted as a file of the app instead of a data: URL, so it works under an img-src 'self' policy.
import tmdbLogoUrl from "@mivo/ui-tokens/assets/tmdb-logo.svg?no-inline";
import { useTranslation } from "react-i18next";

/** Metadata sources that require a visible credit (ADR 0031 section 4). */
export const creditedMetadataSources = ["tmdb", "musicbrainz", "coverartarchive"] as const;

/** Props of {@link MetadataAttribution}. */
export interface MetadataAttributionProps {
  /**
   * Where the shown data came from, e.g. an item's `metadataSources`. Only credited sources
   * render something; unknown values are ignored, so the component renders nothing for "nfo".
   */
  readonly sources: readonly string[];
  readonly className?: string;
}

/**
 * The notice the TMDB terms prescribe word for word. It is legal text, so it is shown in
 * English in every UI language; other languages add their translation below it.
 */
export const tmdbDisclaimer =
  "This product uses the TMDB API but is not endorsed or certified by TMDB.";

const disclaimerLanguage = "en";

function TmdbCredit() {
  const { t, i18n } = useTranslation();
  const english = (i18n.resolvedLanguage ?? disclaimerLanguage).startsWith(disclaimerLanguage);
  return (
    <div className="flex flex-col gap-1">
      <p className="flex items-center gap-2">
        <span>{t("attribution.poweredBy")}</span>
        {/* Smaller than the product's own logo, as the TMDB branding rules require. */}
        <img src={tmdbLogoUrl} alt={t("attribution.tmdbLogo")} className="h-3 w-auto" />
      </p>
      <p lang={disclaimerLanguage}>{tmdbDisclaimer}</p>
      {!english && <p>{t("attribution.tmdbDisclaimerTranslation")}</p>}
    </div>
  );
}

/**
 * Credits the online metadata providers the TMDB and MusicBrainz terms ask for: "Powered by"
 * with the bundled TMDB logo plus the TMDB disclaimer, and name credits for MusicBrainz and
 * the Cover Art Archive. The logo is a bundled asset, so nothing is loaded from TMDB.
 */
export function MetadataAttribution({ sources, className = "" }: MetadataAttributionProps) {
  const { t } = useTranslation();
  const tmdb = sources.includes("tmdb");
  const musicBrainz = sources.includes("musicbrainz");
  const coverArt = sources.includes("coverartarchive");
  if (!tmdb && !musicBrainz && !coverArt) {
    return null;
  }
  return (
    <div className={`flex flex-col gap-2 text-xs text-text-muted ${className}`}>
      {tmdb && <TmdbCredit />}
      {musicBrainz && <p>{t("attribution.musicBrainz")}</p>}
      {coverArt && <p>{t("attribution.coverArtArchive")}</p>}
    </div>
  );
}

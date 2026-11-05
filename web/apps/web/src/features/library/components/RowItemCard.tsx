import type { ItemSummary } from "@mivo/api-client";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { rowCardSizes, type CardFormat } from "../../../shared/lib/rowScroll";
import {
  Badge,
  FilmIcon,
  MediaCard,
  Poster,
  ScreenIcon,
  useFormatEpisodeCode,
  type RowCardProps,
} from "../../../shared/ui";
import { rowCardArtwork, toRowCardModel, type RowCardDetail } from "../lib/rowCard";
import { libraryNamespace } from "../locales";

/** Props of {@link RowItemCard}. */
export interface RowItemCardProps {
  readonly item: ItemSummary;
  readonly format: CardFormat;
  /** The row's tab stop and arrow-key handlers; spread onto the card link. */
  readonly rowCard: RowCardProps;
}

const placeholderIconClass = "size-1/4";
const countBadgeClass =
  "absolute top-1.5 right-1.5 grid h-5.5 min-w-5.5 place-items-center rounded-full bg-accent px-1.5 text-xs font-bold text-on-accent shadow-card transition-opacity group-focus-within:opacity-0 group-hover:opacity-0";

/** Card of one item in a server row (Home, Overview); opens the item detail page. */
export function RowItemCard({ item, format, rowCard }: RowItemCardProps) {
  const { t } = useTranslation(libraryNamespace);
  const card = toRowCardModel(item);
  const artwork = rowCardArtwork(item, format);
  // The details already say "3 new episodes" on a show card; elsewhere the badge says it.
  const badgeLabel =
    card.newEpisodeCount === undefined || card.detail.kind === "newEpisodes"
      ? undefined
      : t("rowCard.newEpisodes", { count: card.newEpisodeCount });
  return (
    <MediaCard
      title={card.title}
      poster={
        <Poster
          imageRef={artwork.imageRef}
          aspect={artwork.aspect}
          alt={card.title}
          sizes={rowCardSizes(format)}
          icon={
            card.isSeries ? (
              <ScreenIcon className={placeholderIconClass} />
            ) : (
              <FilmIcon className={placeholderIconClass} />
            )
          }
        />
      }
      overlay={
        card.newEpisodeCount === undefined ? undefined : (
          <span aria-hidden="true" className={countBadgeClass}>
            {card.newEpisodeCount}
          </span>
        )
      }
      muted={!card.available}
      headingLevel={3}
      renderLink={(linkProps) => (
        <Link to="/items/$itemId" params={{ itemId: card.id }} {...rowCard} {...linkProps} />
      )}
      details={
        <>
          <CardDetail detail={card.detail} />
          {/* In the details, so the link's description reads the count after the season. */}
          {badgeLabel !== undefined && <span className="sr-only">{badgeLabel}</span>}
          {!card.available && <Badge>{t("rowCard.unavailable")}</Badge>}
        </>
      }
    />
  );
}

function CardDetail({ detail }: { readonly detail: RowCardDetail }) {
  const { t } = useTranslation(libraryNamespace);
  const formatEpisodeCode = useFormatEpisodeCode();
  switch (detail.kind) {
    case "newEpisodes":
      return <span>{t("rowCard.newEpisodes", { count: detail.count })}</span>;
    case "show":
      return (
        <>
          {detail.year !== "" && <span>{detail.year}</span>}
          {detail.seasonCount > 0 && (
            <span>{t("rowCard.seasons", { count: detail.seasonCount })}</span>
          )}
        </>
      );
    case "season":
      return <span>{seasonLabel(detail, t)}</span>;
    case "episode": {
      const episodeCode = formatEpisodeCode(detail.numbers);
      return (
        <>
          {episodeCode !== undefined && <span>{episodeCode}</span>}
          {detail.episodeTitle !== "" && <span className="truncate">{detail.episodeTitle}</span>}
        </>
      );
    }
    default:
      return detail.year === "" ? null : <span>{detail.year}</span>;
  }
}

type LibraryT = ReturnType<typeof useTranslation<typeof libraryNamespace>>["t"];

function seasonLabel(detail: Extract<RowCardDetail, { kind: "season" }>, t: LibraryT): string {
  if (detail.seasonNumber === undefined) {
    return detail.name;
  }
  return detail.seasonNumber === 0
    ? t("rowCard.specials")
    : t("rowCard.season", { number: detail.seasonNumber });
}

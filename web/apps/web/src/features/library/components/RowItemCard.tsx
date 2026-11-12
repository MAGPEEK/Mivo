import type { ItemSummary } from "@mivo/api-client";
import { Link } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { rowCardSizes, type CardFormat } from "../../../shared/lib/rowScroll";
import { watchBadges } from "../../../shared/lib/watchState";
import {
  Badge,
  FilmIcon,
  MediaCard,
  type CardLinkProps,
  Poster,
  ScreenIcon,
  useFormatEpisodeCode,
  type RowCardProps,
} from "../../../shared/ui";
import {
  rowCardArtwork,
  toRowCardModel,
  type RowCardArtwork,
  type RowCardDetail,
  type RowCardModel,
} from "../lib/rowCard";
import { libraryNamespace } from "../locales";
import { WatchBadgeOverlay } from "./WatchBadgeOverlay";

/** Props of {@link RowItemCard}. */
export interface RowItemCardProps {
  readonly item: ItemSummary;
  readonly format: CardFormat;
  /** The row's tab stop and arrow-key handlers; spread onto the card link. */
  readonly rowCard: RowCardProps;
  /**
   * Makes the card link start playback at this position in seconds (Continue watching) instead
   * of opening the detail page.
   */
  readonly resumeAt?: number;
  /** Buttons of the card, e.g. an overflow menu; shown on hover or focus. */
  readonly actions?: ReactNode;
  /** Announced when it changes, e.g. why an action failed; empty says nothing. */
  readonly status?: string;
}

const placeholderIconClass = "size-1/4";
const countBadgeClass =
  "absolute top-1.5 right-1.5 grid h-5.5 min-w-5.5 place-items-center rounded-full bg-accent px-1.5 text-xs font-bold text-on-accent shadow-card transition-opacity group-focus-within:opacity-0 group-hover:opacity-0";

/** Card of one item in a server row (Home, Overview); opens the item detail page. */
export function RowItemCard({
  item,
  format,
  rowCard,
  resumeAt,
  actions,
  status,
}: RowItemCardProps) {
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
      poster={<RowCardPoster card={card} artwork={artwork} format={format} />}
      overlay={<RowCardOverlay item={item} newEpisodeCount={card.newEpisodeCount} />}
      actions={actions}
      muted={!card.available}
      headingLevel={3}
      renderLink={(linkProps) => (
        <RowCardLink itemId={card.id} resumeAt={resumeAt} rowCard={rowCard} linkProps={linkProps} />
      )}
      details={
        <>
          <CardDetail detail={card.detail} />
          {/* In the details, so the link's description reads the count after the season. */}
          {badgeLabel !== undefined && <span className="sr-only">{badgeLabel}</span>}
          {!card.available && <Badge>{t("rowCard.unavailable")}</Badge>}
          {status !== undefined && (
            <span role="status" className="sr-only">
              {status}
            </span>
          )}
        </>
      }
    />
  );
}

interface RowCardPosterProps {
  readonly card: RowCardModel;
  readonly artwork: RowCardArtwork;
  readonly format: CardFormat;
}

function RowCardPoster({ card, artwork, format }: RowCardPosterProps) {
  const Icon = card.isSeries ? ScreenIcon : FilmIcon;
  return (
    <Poster
      imageRef={artwork.imageRef}
      aspect={artwork.aspect}
      alt={card.title}
      sizes={rowCardSizes(format)}
      icon={<Icon className={placeholderIconClass} />}
    />
  );
}

interface RowCardLinkProps {
  readonly itemId: string;
  readonly resumeAt: number | undefined;
  readonly rowCard: RowCardProps;
  readonly linkProps: CardLinkProps;
}

// Continue watching plays from the resume position; every other row opens the detail page.
function RowCardLink({ itemId, resumeAt, rowCard, linkProps }: RowCardLinkProps) {
  return resumeAt === undefined ? (
    <Link to="/items/$itemId" params={{ itemId }} {...rowCard} {...linkProps} />
  ) : (
    <Link
      to="/items/$itemId/play"
      params={{ itemId }}
      search={{ t: resumeAt }}
      {...rowCard}
      {...linkProps}
    />
  );
}

interface RowCardOverlayProps {
  readonly item: ItemSummary;
  readonly newEpisodeCount: number | undefined;
}

// The "new episodes" count keeps the corner; the progress bar shows either way.
function RowCardOverlay({ item, newEpisodeCount }: RowCardOverlayProps) {
  const badges = watchBadges({
    kind: item.kind,
    viewerState: item.viewerState,
    durationSeconds: item.media?.durationSeconds,
  });
  return (
    <>
      {newEpisodeCount !== undefined && (
        <span aria-hidden="true" className={countBadgeClass}>
          {newEpisodeCount}
        </span>
      )}
      <WatchBadgeOverlay badges={badges} cornerTaken={newEpisodeCount !== undefined} />
    </>
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

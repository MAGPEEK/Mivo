import type { ItemSummary } from "@mivo/api-client";
import { Link } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { rowCardSizes, type CardFormat } from "../../../shared/lib/rowScroll";
import { watchBadges, type WatchBadges } from "../../../shared/lib/watchState";
import {
  AlertIcon,
  Badge,
  FilmIcon,
  MediaCard,
  Poster,
  ScreenIcon,
  useFormatEpisodeCode,
  type RowCardProps,
} from "../../../shared/ui";
import { useFocusOnMount } from "../hooks/useFocusOnMount";
import type { WatchedToggle } from "../hooks/useWatchedToggle";
import {
  cardArtwork,
  cardEpisodeNumbers,
  toItemCardModel,
  type ItemCardModel,
} from "../lib/itemCard";
import { libraryNamespace } from "../locales";
import { itemPosterSizes } from "./gridLayout";
import { WatchBadgeOverlay } from "./WatchBadgeOverlay";
import { WatchedCardToggle } from "./WatchedCardToggle";

/** Props of {@link ItemCard}. */
export interface ItemCardProps {
  readonly item: ItemSummary;
  /** Focus the card link when it appears, e.g. the first card of a newly loaded page. */
  readonly focusOnMount?: boolean;
  /** Card shape: poster 2:3 (default), episode still 16:9 or square 1:1. */
  readonly format?: CardFormat;
  /**
   * Season of an episode, for its `S3 · E9` label; rows know it from their source while the
   * item summary does not carry it.
   */
  readonly seasonNumber?: number;
  /** Replaces the watch-state badges over the artwork. */
  readonly overlay?: ReactNode;
  /**
   * Adds the hover toggle "Mark as watched" and shows its expected state in the badges while
   * a request runs. Without it the card only shows the state of `item`.
   */
  readonly watchedToggle?: WatchedToggle | undefined;
  /** Set when the card sits in a `MediaRow`: the row's tab stop and a fixed card width. */
  readonly rowCard?: RowCardProps;
}

const placeholderIconClass = "size-1/4";

/** Card of one item in a grid or row; opens the item detail page. */
export function ItemCard({
  item,
  focusOnMount = false,
  format = "poster",
  seasonNumber,
  overlay,
  watchedToggle,
  rowCard,
}: ItemCardProps) {
  const linkRef = useFocusOnMount<HTMLAnchorElement>(focusOnMount);
  const formatEpisodeCode = useFormatEpisodeCode();
  const card = toItemCardModel(item);
  const artwork = cardArtwork(item, format);
  const episodeNumbers = cardEpisodeNumbers(item, seasonNumber);
  const episodeCode = episodeNumbers === undefined ? undefined : formatEpisodeCode(episodeNumbers);
  return (
    <MediaCard
      title={card.title}
      poster={
        <Poster
          imageRef={artwork.imageRef}
          aspect={artwork.aspect}
          alt={card.title}
          sizes={rowCard === undefined ? itemPosterSizes : rowCardSizes(format)}
          icon={<PlaceholderIcon isShow={card.isShow} />}
        />
      }
      overlay={overlay ?? <WatchBadgeOverlay badges={cardBadges(item, watchedToggle)} />}
      actions={
        watchedToggle === undefined ? undefined : (
          <WatchedCardToggle title={card.title} toggle={watchedToggle} />
        )
      }
      muted={!card.available}
      headingLevel={rowCard === undefined ? 2 : 3}
      renderLink={(linkProps) => (
        <Link
          to="/items/$itemId"
          params={{ itemId: card.id }}
          ref={linkRef}
          {...rowCard}
          {...linkProps}
        />
      )}
      details={<CardDetails card={card} episodeCode={episodeCode} />}
    />
  );
}

function PlaceholderIcon({ isShow }: { readonly isShow: boolean }) {
  return isShow ? (
    <ScreenIcon className={placeholderIconClass} />
  ) : (
    <FilmIcon className={placeholderIconClass} />
  );
}

// With a toggle the badges follow its state, which is the expected one while a request runs.
function cardBadges(item: ItemSummary, watchedToggle: WatchedToggle | undefined): WatchBadges {
  return watchBadges({
    kind: item.kind,
    viewerState: watchedToggle === undefined ? item.viewerState : watchedToggle.viewerState,
    durationSeconds: item.media?.durationSeconds,
  });
}

interface CardDetailsProps {
  readonly card: ItemCardModel;
  readonly episodeCode: string | undefined;
}

function CardDetails({ card, episodeCode }: CardDetailsProps) {
  const { t } = useTranslation(libraryNamespace);
  return (
    <>
      {episodeCode !== undefined && <span>{episodeCode}</span>}
      {card.year !== "" && <span>{card.year}</span>}
      {card.isShow ? (
        <span>{t("items.seasons", { count: card.seasonCount })}</span>
      ) : (
        <>
          {card.duration !== "" && <span>{card.duration}</span>}
          {card.chip !== "" && <Badge>{card.chip}</Badge>}
          {!card.available && (
            <Badge icon={<AlertIcon className="size-3 shrink-0 text-accent" />}>
              {t("items.unavailable")}
            </Badge>
          )}
        </>
      )}
      {card.unmatched && <Badge>{t("items.unmatched")}</Badge>}
    </>
  );
}

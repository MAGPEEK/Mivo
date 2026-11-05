import type { ImageRef, ItemSummary } from "@mivo/api-client";

import type { EpisodeNumbers } from "../../../shared/lib/episodeCode";
import type { CardFormat } from "../../../shared/lib/rowScroll";
import type { PosterAspect } from "../../../shared/ui";

/** What the line below a row card's title says, by the kind of item. */
export type RowCardDetail =
  | { readonly kind: "newEpisodes"; readonly count: number }
  | { readonly kind: "show"; readonly year: string; readonly seasonCount: number }
  | { readonly kind: "season"; readonly seasonNumber: number | undefined; readonly name: string }
  | {
      readonly kind: "episode";
      readonly numbers: EpisodeNumbers;
      /** The episode's own title when the card is named after its show; else "". */
      readonly episodeTitle: string;
    }
  | { readonly kind: "other"; readonly year: string };

/** What a card in a server row (Home, Overview) shows for one item. */
export interface RowCardModel {
  readonly id: string;
  /** Name of the card and its link: the show for seasons and episodes, else the item. */
  readonly title: string;
  readonly detail: RowCardDetail;
  /** Number for the "new episodes" badge of grouped show and season cards (HO-05). */
  readonly newEpisodeCount: number | undefined;
  /** True for shows, seasons and episodes: their placeholder is a screen, not a film. */
  readonly isSeries: boolean;
  /** False for a movie or episode whose file is unavailable; true for containers. */
  readonly available: boolean;
}

/** The image and box shape of a card in one format. */
export interface RowCardArtwork {
  readonly imageRef: ImageRef | undefined;
  readonly aspect: PosterAspect;
}

const seriesKinds: ReadonlySet<string> = new Set(["show", "season", "episode"]);
const fileKinds: ReadonlySet<string> = new Set(["movie", "episode"]);

function yearText(year: number | undefined): string {
  return year === undefined ? "" : String(year);
}

function nonEmpty(value: string | undefined): string | undefined {
  return value === undefined || value.trim() === "" ? undefined : value;
}

function cardDetail(item: ItemSummary, showTitle: string | undefined): RowCardDetail {
  switch (item.kind) {
    case "show":
      return item.newEpisodeCount === undefined
        ? { kind: "show", year: yearText(item.year), seasonCount: item.childCount ?? 0 }
        : { kind: "newEpisodes", count: item.newEpisodeCount };
    case "season":
      return { kind: "season", seasonNumber: item.number ?? item.seasonNumber, name: item.title };
    case "episode":
      return {
        kind: "episode",
        numbers: { season: item.seasonNumber, number: item.number, numberEnd: item.numberEnd },
        episodeTitle: showTitle === undefined ? "" : item.title,
      };
    default:
      return { kind: "other", year: yearText(item.year) };
  }
}

/**
 * Derives the card of `item` in a server row (Home, Overview). Grouped season and episode cards are named after
 * their show, as the row cards of the approved prototype; the season or episode follows below.
 */
export function toRowCardModel(item: ItemSummary): RowCardModel {
  const isSeries = seriesKinds.has(item.kind);
  const showTitle =
    item.kind === "season" || item.kind === "episode" ? nonEmpty(item.showTitle) : undefined;
  return {
    id: item.id,
    title: showTitle ?? item.title,
    detail: cardDetail(item, showTitle),
    newEpisodeCount:
      item.kind === "show" || item.kind === "season" ? item.newEpisodeCount : undefined,
    isSeries,
    // Containers have no file of their own (API contract: `ItemSummary.media`).
    available: !fileKinds.has(item.kind) || (item.media?.available ?? false),
  };
}

const formatAspects: Readonly<Record<CardFormat, PosterAspect>> = {
  poster: "poster",
  episode: "still",
  square: "square",
};

/**
 * The artwork of `item` in a row card `format`: episode cards show the still, square cards
 * the square image, both falling back to the poster.
 */
export function rowCardArtwork(item: ItemSummary, format: CardFormat): RowCardArtwork {
  const formatImages: Readonly<Record<CardFormat, ImageRef | undefined>> = {
    poster: item.poster,
    episode: item.thumb,
    square: item.square,
  };
  return { imageRef: formatImages[format] ?? item.poster, aspect: formatAspects[format] };
}

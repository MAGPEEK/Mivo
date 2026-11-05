import type { ImageRef, ItemKind, ItemSummary } from "@mivo/api-client";

import { episodeRange, type EpisodeRange } from "../../../shared/lib/episodeCode";
import { durationParts, type DurationParts } from "./duration";

/** The translated texts a child row needs; the hook builds them from i18next. */
export interface ChildRowTexts {
  /** "Season 3". */
  readonly seasonName: (number: number) => string;
  /** The name of season 0. */
  readonly specials: string;
  /** "Season 1 · 9 episodes". */
  readonly seasonCaption: (name: string, episodeCount: number) => string;
  /** "Episode 8" or "Episodes 1–2". */
  readonly episodeNumber: (range: EpisodeRange) => string;
  readonly duration: (parts: DurationParts) => string;
}

/** What one child of a show or season shows in the list. */
export interface ChildRow {
  readonly id: string;
  readonly kind: ItemKind;
  /** Link text: the season caption, or the episode title. */
  readonly title: string;
  /** `Episode 8`; empty for unnumbered episodes and seasons. */
  readonly label: string;
  /** Runtime of the primary file; empty when unknown. */
  readonly runtime: string;
  /** Season poster or episode still; `undefined` shows the placeholder. */
  readonly image: ImageRef | undefined;
}

function seasonName(item: ItemSummary, texts: ChildRowTexts): string {
  if (item.number === undefined) {
    return item.title;
  }
  return item.number === 0 ? texts.specials : texts.seasonName(item.number);
}

/** The display content of one child (season or episode) of a container. */
export function toChildRow(item: ItemSummary, texts: ChildRowTexts): ChildRow {
  if (item.kind === "season") {
    return {
      id: item.id,
      kind: item.kind,
      title: texts.seasonCaption(seasonName(item, texts), item.childCount ?? 0),
      label: "",
      runtime: "",
      image: item.poster,
    };
  }
  const range = episodeRange(item.number, item.numberEnd);
  const seconds = item.media?.durationSeconds;
  return {
    id: item.id,
    kind: item.kind,
    title: item.title,
    label: range === undefined ? "" : texts.episodeNumber(range),
    runtime: seconds === undefined ? "" : texts.duration(durationParts(seconds)),
    image: item.thumb,
  };
}

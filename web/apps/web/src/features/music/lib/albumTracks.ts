import type { ItemSummary } from "@mivo/api-client";

import { formatDuration } from "../../library";

/** What one row of an album's track list shows. */
export interface TrackRowModel {
  readonly id: string;
  readonly title: string;
  /** Track number on its disc, e.g. `3`; "" when the track has none. */
  readonly number: string;
  /** Clock notation such as `3:12`; "" when unknown. */
  readonly duration: string;
  /** False when the file could not be probed: the row offers no Play. */
  readonly available: boolean;
}

/** The tracks of one disc, in server order (track number, then file name). */
export interface DiscGroup {
  /** Disc number; `undefined` for tracks without a disc tag. */
  readonly disc: number | undefined;
  readonly tracks: readonly TrackRowModel[];
}

/** Derives the row content of a track summary. */
export function toTrackRowModel(track: ItemSummary): TrackRowModel {
  const seconds = track.media?.durationSeconds;
  return {
    id: track.id,
    title: track.title,
    number: track.number === undefined ? "" : String(track.number),
    duration: seconds === undefined ? "" : formatDuration(seconds),
    available: track.media?.available ?? false,
  };
}

// Missing numbers sort after every real one.
function numberOrLast(value: number | undefined): number {
  return value ?? Number.POSITIVE_INFINITY;
}

/**
 * The tracks in album order: by disc, then track number; tracks without a disc or number come
 * last. The server's children order is by track number only, so discs would interleave.
 * Array sort is stable, so equal tracks keep the server's order.
 */
export function sortTracks(tracks: readonly ItemSummary[]): ItemSummary[] {
  return [...tracks].sort(
    (a, b) =>
      compareNumbers(numberOrLast(a.discNumber), numberOrLast(b.discNumber)) ||
      compareNumbers(numberOrLast(a.number), numberOrLast(b.number)),
  );
}

// Subtraction would yield NaN for two infinities.
function compareNumbers(a: number, b: number): number {
  return a === b ? 0 : a < b ? -1 : 1;
}

/**
 * Groups the tracks of an album by disc, in {@link sortTracks} order: consecutive tracks of the
 * same disc form one group.
 */
export function groupByDisc(tracks: readonly ItemSummary[]): DiscGroup[] {
  const groups: { disc: number | undefined; tracks: TrackRowModel[] }[] = [];
  for (const track of sortTracks(tracks)) {
    const last = groups.at(-1);
    if (last !== undefined && last.disc === track.discNumber) {
      last.tracks.push(toTrackRowModel(track));
    } else {
      groups.push({ disc: track.discNumber, tracks: [toTrackRowModel(track)] });
    }
  }
  return groups;
}

/** Whether the album spans several discs, so the list names each disc. */
export function hasSeveralDiscs(groups: readonly DiscGroup[]): boolean {
  return groups.length > 1;
}

/** The first track of the album that can be played: what the album's Play starts. */
export function firstPlayableTrackId(tracks: readonly ItemSummary[]): string | undefined {
  return playQueueIds(tracks)[0];
}

/** The IDs of the playable tracks in album order: the queue Play and Shuffle hand to the player. */
export function playQueueIds(tracks: readonly ItemSummary[]): string[] {
  return sortTracks(tracks)
    .filter((track) => track.media?.available === true)
    .map((track) => track.id);
}

/** The track a shuffle starts with, picked by `fraction` in [0, 1); none for an empty queue. */
export function shuffleStartId(ids: readonly string[], fraction: number): string | undefined {
  return ids[Math.floor(fraction * ids.length)];
}

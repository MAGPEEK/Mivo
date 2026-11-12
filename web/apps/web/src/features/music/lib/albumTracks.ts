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
  /** Disc number; tracks without a disc tag count as disc 1, like on the server. */
  readonly disc: number;
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

// The server orders tracks without a disc tag as disc 1 (ListChildren of an album), so the
// grouping does the same; otherwise the list would split what the server keeps together.
const untaggedDisc = 1;

/**
 * Groups the tracks of an album by disc. The server lists them by disc, track number (unnumbered
 * last), file name and ID, also across pages, so consecutive tracks of a disc form one group.
 */
export function groupByDisc(tracks: readonly ItemSummary[]): DiscGroup[] {
  const groups: { disc: number; tracks: TrackRowModel[] }[] = [];
  for (const track of tracks) {
    const disc = track.discNumber ?? untaggedDisc;
    const last = groups.at(-1);
    if (last?.disc === disc) {
      last.tracks.push(toTrackRowModel(track));
    } else {
      groups.push({ disc, tracks: [toTrackRowModel(track)] });
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

/** The IDs of the playable tracks in server (album) order: the queue Play and Shuffle hand over. */
export function playQueueIds(tracks: readonly ItemSummary[]): string[] {
  return tracks.filter((track) => track.media?.available === true).map((track) => track.id);
}

/** The track a shuffle starts with, picked by `fraction` in [0, 1); none for an empty queue. */
export function shuffleStartId(ids: readonly string[], fraction: number): string | undefined {
  return ids[Math.floor(fraction * ids.length)];
}

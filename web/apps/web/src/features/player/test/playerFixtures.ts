// Fixtures of the player feature: client profiles, an item with one file and sessions shaped
// like the server's answers (ADR 0024).

import type { ClientProfile, ItemDetail, PlaybackSession } from "@mivo/api-client";

/** ID of the item every player test opens. */
export const playerItemId = "item-1";

/** ID of the item's only file. */
export const playerFileId = "file-1";

/** Probed duration of the item's file in seconds. */
export const playerFileDurationSeconds = 120;

const profileCodecs = { videoCodecs: ["h264", "vp9"], audioCodecs: ["aac", "opus"] };

/** A desktop browser that plays HLS through hls.js (Media Source Extensions). */
export const mseProfile: ClientProfile = {
  name: "Test browser",
  directPlay: [{ container: "mp4", ...profileCodecs }],
  hls: { delivery: "mse", ...profileCodecs },
};

/** A browser that plays HLS with the media element itself (iOS Safari). */
export const nativeHlsProfile: ClientProfile = {
  ...mseProfile,
  hls: { delivery: "native", ...profileCodecs },
};

/** A browser without any HLS support: only direct play is possible. */
export const noHlsProfile: ClientProfile = {
  ...mseProfile,
  hls: { delivery: "none", videoCodecs: [], audioCodecs: [] },
};

/** The item the player opens, with one playable file. */
export function playerItem(overrides: Partial<ItemDetail> = {}): ItemDetail {
  return {
    id: playerItemId,
    libraryId: "library-1",
    title: "Big Buck Bunny",
    files: [
      {
        id: playerFileId,
        fileName: "Big.Buck.Bunny.mp4",
        sizeBytes: 1_000_000,
        modifiedAt: "2026-10-01T09:30:00Z",
        available: true,
        container: "mp4",
        durationSeconds: playerFileDurationSeconds,
        streams: [],
      },
    ],
    ...overrides,
  };
}

/** Fields of a fake session a test usually varies. */
export interface SessionFields {
  readonly id: string;
  readonly delivery?: PlaybackSession["delivery"];
  readonly startPositionSeconds?: number;
  readonly state?: PlaybackSession["state"];
}

/** A session the fake server returns: direct play unless `delivery` says `hls`. */
export function fakeSession(fields: SessionFields): PlaybackSession {
  const { id, delivery = "file", startPositionSeconds = 0, state = "ready" } = fields;
  const stream = delivery === "file" ? "file" : "hls/index.m3u8";
  return {
    id,
    fileId: playerFileId,
    state,
    mode: delivery === "file" ? "direct_play" : "remux",
    delivery,
    streamUrl: `/api/v1/playback/sessions/${id}/${stream}`,
    startPositionSeconds,
    durationSeconds: playerFileDurationSeconds,
    sourceContainer: "mp4",
    targetContainer: "mp4",
    streams: [],
    reasons: [],
    warnings: [],
  };
}

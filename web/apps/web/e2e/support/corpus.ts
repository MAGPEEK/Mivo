// The small test corpus the playback specs scan and play, generated with the repository's
// corpus generator (scripts/gen-testmedia) into the library of the E2E server.
import { existsSync } from "node:fs";
import { join } from "node:path";

import { assertCommandAvailable, runCommand } from "./commands";

/** One generated clip and what the server and the browser make of it. */
export interface CorpusClip {
  /** gen-testmedia case name. */
  readonly caseName: string;
  /** Path below the output directory, as gen-testmedia writes it. */
  readonly relativePath: string;
  /**
   * Title the scanner derives from the file name ("." and "_" become spaces). Two clips may
   * share it, e.g. `h264_aac.mkv` and `h264_aac.flv`.
   */
  readonly title: string;
  /** Technical chip of the clip's card, which tells clips with the same title apart. */
  readonly chip: string;
}

/** VP9/Opus in WebM: open codecs, Direct Play in every Chromium. */
export const webmClip: CorpusClip = {
  caseName: "video-webm-vp9-opus",
  relativePath: "video/vp9_opus.webm",
  title: "vp9 opus",
  chip: "WebM · VP9 · Opus",
};

/** H.264/AAC in MP4: Direct Play where the browser has H.264. */
export const mp4Clip: CorpusClip = {
  caseName: "video-mp4-h264-aac-faststart",
  relativePath: "video/h264_aac_faststart.mp4",
  title: "h264 aac faststart",
  chip: "MP4 · H.264 · AAC",
};

/**
 * H.264/AAC in Matroska: Direct Play in a browser that plays Matroska (Chrome and Edge 156),
 * else the server remuxes it to HLS.
 */
export const mkvClip: CorpusClip = {
  caseName: "video-mkv-h264-aac",
  relativePath: "video/h264_aac.mkv",
  title: "h264 aac",
  chip: "MKV · H.264 · AAC",
};

/** H.264/AAC in FLV: no browser plays FLV, so a browser with H.264 gets a remux. */
export const flvClip: CorpusClip = {
  caseName: "video-flv-h264-aac",
  relativePath: "video/h264_aac.flv",
  title: "h264 aac",
  chip: "FLV · H.264 · AAC",
};

/** MPEG-4 Part 2 (Xvid)/MP3 in AVI: no browser decodes Xvid, so the server transcodes. */
export const aviClip: CorpusClip = {
  caseName: "video-avi-xvid-mp3",
  relativePath: "video/mpeg4_xvid_mp3.avi",
  title: "mpeg4 xvid mp3",
  chip: "AVI · MPEG-4 · MP3",
};

/**
 * The 30-second seek clip (H.264/AAC in Matroska, a keyframe every 2 s). The other clips last
 * 2 s, which the browser buffers at once, so a seek in them never needs a new session.
 */
export const longMkvClip: CorpusClip = {
  caseName: "video-mkv-h264-aac-long",
  relativePath: "video/h264_aac_long.mkv",
  title: "h264 aac long",
  chip: "MKV · H.264 · AAC",
};

/** Every clip of the E2E corpus. */
export const corpusClips: readonly CorpusClip[] = [
  webmClip,
  mp4Clip,
  mkvClip,
  flvClip,
  aviClip,
  longMkvClip,
];

// go run compiles the generator first; FFmpeg then encodes the small clips in parallel.
const generateTimeoutMs = 5 * 60_000;

/**
 * Fails with a clear message when FFmpeg or ffprobe is missing: the corpus generator needs
 * FFmpeg, and the server needs ffprobe to scan and FFmpeg to remux and transcode.
 */
export function assertFFmpegAvailable(): void {
  for (const tool of ["ffmpeg", "ffprobe"]) {
    assertCommandAvailable(
      tool,
      ["-version"],
      `${tool} was not found on PATH. The E2E suite generates its test videos with FFmpeg and ` +
        "the server scans, remuxes and transcodes them with it; install FFmpeg (see " +
        "docs/development.md) and open a new shell.",
    );
  }
}

/**
 * Generates {@link corpusClips} into `outputDirectory`.
 * @throws Error when the generator fails or skipped a clip, e.g. because the installed FFmpeg
 *   lacks an encoder.
 */
export async function generateCorpus(outputDirectory: string): Promise<void> {
  const only = corpusClips.map((clip) => clip.caseName).join(",");
  await runCommand(
    "go",
    ["run", "./scripts/gen-testmedia", "-only", only, "-out", outputDirectory],
    { timeoutMs: generateTimeoutMs },
  );
  const missing = corpusClips.filter(
    (clip) => !existsSync(join(outputDirectory, clip.relativePath)),
  );
  if (missing.length > 0) {
    throw new Error(
      `gen-testmedia skipped ${missing.map((clip) => clip.caseName).join(", ")}; the installed ` +
        `FFmpeg probably lacks an encoder. See ${join(outputDirectory, "manifest.json")}.`,
    );
  }
}

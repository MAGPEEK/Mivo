// Display labels for stream properties: channels, dynamic range, bit depth and resolution.
// Technical notation, the same in every language.

const channelLabels: ReadonlyMap<number, string> = new Map([
  [1, "1.0"],
  [2, "2.0"],
  [3, "2.1"],
  [4, "4.0"],
  [5, "5.0"],
  [6, "5.1"],
  [7, "6.1"],
  [8, "7.1"],
]);

/** Label of an audio channel count, e.g. 2 → `2.0`, 6 → `5.1`; other counts as the number. */
export function channelLabel(channels: number): string {
  return channelLabels.get(channels) ?? String(channels);
}

/** Absent HDR metadata means standard dynamic range. */
const standardDynamicRangeLabel = "SDR";

/** Label of an HDR format (`hdr10` → `HDR10`, `hlg` → `HLG`); `undefined` is SDR video. */
export function hdrLabel(hdr: string | undefined): string {
  return hdr === undefined ? standardDynamicRangeLabel : hdr.toUpperCase();
}

/** Label of a video bit depth, e.g. 10 → `10-bit`. */
export function bitDepthLabel(bitDepth: number): string {
  return `${String(bitDepth)}-bit`;
}

interface ResolutionTier {
  readonly height: number;
  readonly label: string;
}

// Highest first. A source belongs to the first tier it reaches within the tolerance, so encodes
// that crop a few lines (1920×1072, 3840×2076) keep their nominal tier.
const resolutionTiers: readonly ResolutionTier[] = [
  { height: 4320, label: "8K" },
  { height: 2160, label: "4K" },
  { height: 1440, label: "1440p" },
  { height: 1080, label: "1080p" },
  { height: 720, label: "720p" },
  { height: 576, label: "576p" },
  { height: 540, label: "540p" },
  { height: 480, label: "480p" },
];
const tierTolerance = 0.95;
const widescreenHeightPerWidth = 9 / 16;

/**
 * Label of a video resolution: 1080 → `1080p`, 2160 → `4K`; heights below every tier are shown
 * as they are (360 → `360p`). Pass the width as well, so letterboxed films classify by their
 * frame (1920×800 is `1080p`, not `720p`) and portrait video by its shorter side (1080×1920 is
 * `1080p`). A height that is not a positive number gives an empty label.
 */
export function resolutionLabel(height: number, width?: number): string {
  if (!isPositive(height)) {
    return "";
  }
  const nominalHeight = nominalFrameHeight(height, width);
  const tier = resolutionTiers.find(
    (candidate) => nominalHeight >= candidate.height * tierTolerance,
  );
  return tier?.label ?? `${String(Math.round(nominalHeight))}p`;
}

/** The height a landscape 16:9 frame of the same size would have; an unusable width is ignored. */
function nominalFrameHeight(height: number, width: number | undefined): number {
  if (width === undefined || !isPositive(width)) {
    return height;
  }
  const shorterSide = Math.min(height, width);
  const longerSide = Math.max(height, width);
  return Math.max(shorterSide, longerSide * widescreenHeightPerWidth);
}

function isPositive(value: number): boolean {
  return Number.isFinite(value) && value > 0;
}

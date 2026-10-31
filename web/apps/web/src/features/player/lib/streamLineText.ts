import type { StreamLine } from "@mivo/player-core";

/**
 * The technical parts of a stream line in the overlay: the codec (with an arrow when it
 * changes), the target resolution and the target channels, e.g. `HEVC → H.264`, `1080p`.
 * Labels read the same in every language; the app adds the translated kind and action.
 */
export function streamLineParts(line: StreamLine): string[] {
  const codec =
    line.sourceCodec === line.targetCodec
      ? line.targetCodec
      : `${line.sourceCodec} → ${line.targetCodec}`;
  return [codec, line.targetResolution, line.targetChannels].filter(
    (part): part is string => part !== undefined,
  );
}

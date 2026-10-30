import type { CodecLimit } from "./clientProfile";
import type { CodecVariant } from "./probeTables";

/** The playing variants of every codec that plays in one probe context, in context order. */
export type PlayingCodecs = ReadonlyMap<string, readonly CodecVariant[]>;

/**
 * For each codec, the variants that play in every context where the codec plays at all. The
 * profile carries one limit per codec for direct play and HLS alike, so the strictest context
 * wins (fail closed, like the Go engine): a 10-bit variant that plays progressively but not
 * through MSE must not lift the HLS limit. An empty list means the contexts share no variant.
 * Codecs that play nowhere are left out; the result follows the order of `codecs`.
 */
export function commonVariants(
  codecs: readonly string[],
  contexts: readonly PlayingCodecs[],
): Map<string, readonly CodecVariant[]> {
  const common = new Map<string, readonly CodecVariant[]>();
  for (const codec of codecs) {
    const perContext = contexts.flatMap((context) => {
      const variants = context.get(codec);
      return variants === undefined ? [] : [variants];
    });
    if (perContext.length > 0) {
      common.set(
        codec,
        perContext.reduce((kept, variants) => kept.filter((variant) => variants.includes(variant))),
      );
    }
  }
  return common;
}

/**
 * Derives the per-codec limits from the variants that play everywhere. A codec whose variants
 * carry bit depths gets the highest common depth; one whose variants carry profile names gets
 * the common profiles. Codecs without such details, or without common variants, get no entry.
 */
export function deriveCodecLimits(
  common: ReadonlyMap<string, readonly CodecVariant[]>,
): CodecLimit[] {
  return [...common].flatMap(([codec, variants]) => {
    const limit = limitFor(codec, variants);
    return limit === undefined ? [] : [limit];
  });
}

function limitFor(codec: string, playing: readonly CodecVariant[]): CodecLimit | undefined {
  const bitDepths = playing.flatMap(({ bitDepth }) => (bitDepth === undefined ? [] : [bitDepth]));
  const profiles = playing.flatMap(({ profile }) => (profile === undefined ? [] : [profile]));
  if (bitDepths.length === 0 && profiles.length === 0) {
    return undefined;
  }
  const limit: CodecLimit = { codec };
  if (bitDepths.length > 0) {
    limit.maxBitDepth = Math.max(...bitDepths);
  }
  if (profiles.length > 0) {
    limit.profiles = profiles;
  }
  return limit;
}

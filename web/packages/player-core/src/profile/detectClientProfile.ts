import type { ClientProfile, DirectPlayProfile, HlsDelivery } from "./clientProfile";
import { commonVariants, deriveCodecLimits, type PlayingCodecs } from "./codecLimits";
import { createDecodingCheck, type DecodingCheck, type MediaKind } from "./decodingCheck";
import type { ClientProbes } from "./probes";
import {
  audioCodecIds,
  audioCodecVariants,
  containerProbes,
  hlsPlaylistMimeType,
  hlsSegmentProbe,
  videoCodecIds,
  videoCodecVariants,
  type AudioCodecId,
  type CodecVariant,
  type ContainerProbe,
  type ProbeContext,
  type VideoCodecId,
} from "./probeTables";

/** Longest client name the API accepts (`ClientProfile.name`), in characters. */
export const maxClientNameLength = 64;

// The server rejects control characters in the name (400), so they never leave the client.
const controlCharacters = /\p{Cc}/gu;

/** Options of {@link detectClientProfile}. */
export interface DetectClientProfileOptions {
  /**
   * Client name for logs and the overlay; control characters are removed and the rest is cut
   * to {@link maxClientNameLength} characters.
   */
  readonly name: string;
}

/** One way of asking whether a content type plays. */
interface ProbeSurface {
  readonly supportsType: (contentType: string) => boolean;
  readonly decodes: (contentType: string, kind: MediaKind) => Promise<boolean>;
}

interface CodecVariants {
  readonly codec: string;
  readonly variants: readonly CodecVariant[];
}

interface CodecQuery {
  readonly kind: MediaKind;
  readonly mimeType: string;
  readonly codecs: readonly CodecVariants[];
}

interface ProbedContext {
  readonly video: PlayingCodecs;
  readonly audio: PlayingCodecs;
}

interface ProbedContainer extends ProbedContext {
  readonly container: string;
}

interface ProbedHls extends ProbedContext {
  readonly delivery: HlsDelivery;
}

const allCodecIds: readonly string[] = [...videoCodecIds, ...audioCodecIds];

/**
 * Detects what this client plays (ADR 0023): progressive containers through `canPlayType`, HLS
 * through Media Source Extensions or native playback, and codec limits from bit-depth and
 * profile variants. With Media Capabilities available, a codec must also decode. Browsers are
 * never special-cased; whatever the probes answer is what the profile says.
 */
export async function detectClientProfile(
  probes: ClientProbes,
  options: DetectClientProfileOptions,
): Promise<ClientProfile> {
  const decodes = createDecodingCheck(probes);
  const progressive = progressiveSurface(probes, decodes);
  const [containers, hls] = await Promise.all([
    Promise.all(playableContainers(probes).map((probe) => probeContainer(probe, progressive))),
    probeHls(probes, decodes),
  ]);
  const common = commonVariants(
    allCodecIds,
    [...containers, hls].flatMap(({ video, audio }) => [video, audio]),
  );
  // A codec whose contexts share no variant plays nowhere reliably, so it is not listed at all.
  const reliable = new Set(
    [...common].filter(([, variants]) => variants.length > 0).map(([codec]) => codec),
  );
  const listed = (playing: PlayingCodecs) =>
    [...playing.keys()].filter((codec) => reliable.has(codec));
  const directPlay = containers.flatMap(({ container, video, audio }): DirectPlayProfile[] => {
    const videoCodecs = listed(video);
    return videoCodecs.length > 0 ? [{ container, videoCodecs, audioCodecs: listed(audio) }] : [];
  });
  return {
    name: sanitizeName(options.name),
    directPlay,
    hls: { delivery: hls.delivery, videoCodecs: listed(hls.video), audioCodecs: listed(hls.audio) },
    codecLimits: deriveCodecLimits(common),
  };
}

/** `canPlayType` answers `probably` and `maybe` both mean "plays" (ADR 0023). */
function isPlayableAnswer(answer: string): boolean {
  return answer === "probably" || answer === "maybe";
}

function progressiveSurface(probes: ClientProbes, decodes: DecodingCheck): ProbeSurface {
  return {
    supportsType: (contentType) => isPlayableAnswer(probes.mediaElement.canPlayType(contentType)),
    decodes: (contentType, kind) => decodes("file", contentType, kind),
  };
}

/** Containers whose bare video MIME type plays; the others are not probed further. */
function playableContainers(probes: ClientProbes): ContainerProbe[] {
  return containerProbes.filter(({ videoMimeType }) =>
    isPlayableAnswer(probes.mediaElement.canPlayType(videoMimeType)),
  );
}

async function probeContainer(
  probe: ContainerProbe,
  surface: ProbeSurface,
): Promise<ProbedContainer> {
  return { container: probe.container, ...(await probeContext(probe, surface)) };
}

/**
 * MSE wins whenever it plays an HLS video codec: browsers such as Chrome answer the playlist
 * type as well, but hls.js gives the player control over buffering and restarts. Native HLS is
 * used only without a usable MSE (iOS Safari).
 */
async function probeHls(probes: ClientProbes, decodes: DecodingCheck): Promise<ProbedHls> {
  const { mediaElement, mediaSource } = probes;
  const viaMediaSource =
    mediaSource === undefined
      ? undefined
      : await probeContext(hlsSegmentProbe, {
          supportsType: (contentType) => mediaSource.isTypeSupported(contentType),
          decodes: (contentType, kind) => decodes("media-source", contentType, kind),
        });
  if (viaMediaSource !== undefined && viaMediaSource.video.size > 0) {
    return { delivery: "mse", ...viaMediaSource };
  }
  if (isPlayableAnswer(mediaElement.canPlayType(hlsPlaylistMimeType))) {
    const native = await probeContext(hlsSegmentProbe, progressiveSurface(probes, decodes));
    return { delivery: "native", ...native };
  }
  return viaMediaSource === undefined
    ? { delivery: "none", video: new Map(), audio: new Map() }
    : { delivery: "mse", ...viaMediaSource };
}

async function probeContext(context: ProbeContext, surface: ProbeSurface): Promise<ProbedContext> {
  const [video, audio] = await Promise.all([
    probeCodecs(surface, {
      kind: "video",
      mimeType: context.videoMimeType,
      codecs: context.videoCodecs.map(videoCodec),
    }),
    probeCodecs(surface, {
      kind: "audio",
      mimeType: context.audioMimeType,
      codecs: context.audioCodecs.map(audioCodec),
    }),
  ]);
  return { video, audio };
}

/** A codec plays when any of its variants plays; the result keeps only playing codecs. */
async function probeCodecs(surface: ProbeSurface, query: CodecQuery): Promise<PlayingCodecs> {
  const results = await Promise.all(
    query.codecs.map(async ({ codec, variants }) => {
      const verdicts = await Promise.all(
        variants.map((variant) =>
          variantPlays(surface, contentType(query.mimeType, variant), query.kind),
        ),
      );
      return [codec, variants.filter((_, index) => verdicts[index] === true)] as const;
    }),
  );
  return new Map(results.filter(([, playing]) => playing.length > 0));
}

function variantPlays(surface: ProbeSurface, type: string, kind: MediaKind): Promise<boolean> {
  return surface.supportsType(type) ? surface.decodes(type, kind) : Promise.resolve(false);
}

function contentType(mimeType: string, variant: CodecVariant): string {
  return `${mimeType}; codecs="${variant.codecString}"`;
}

function videoCodec(codec: VideoCodecId): CodecVariants {
  return { codec, variants: videoCodecVariants[codec] };
}

function audioCodec(codec: AudioCodecId): CodecVariants {
  return { codec, variants: audioCodecVariants[codec] };
}

/** Removes control characters, then cuts by code points so no surrogate pair is split. */
function sanitizeName(name: string): string {
  return Array.from(name.replace(controlCharacters, "")).slice(0, maxClientNameLength).join("");
}

// Fake probe answers that reproduce what desktop browsers report (OD-6: Chrome/Edge and Firefox
// are release targets, Safari is best effort). Codec strings are written out instead of being
// read from the probe tables, so the tests also pin the strings of ADR 0023.
import type { ClientProbes, DecodingConfiguration, DecodingResult } from "../probes";

type PlayableAnswer = "probably" | "maybe";

/** The answers of one fake browser. */
export interface FakeBrowserAnswers {
  /** Full MIME type (with codecs parameter where probed) → non-empty `canPlayType` answer. */
  readonly canPlayType: Readonly<Record<string, PlayableAnswer>>;
  /** Content types `MediaSource.isTypeSupported` accepts; absent means no MSE. */
  readonly mediaSourceTypes?: readonly string[];
  /** Whether `navigator.mediaCapabilities` exists. */
  readonly mediaCapabilities?: {
    /** Content types decodingInfo reports as unsupported although the type check passed. */
    readonly unsupported?: readonly string[];
    /** Content types decodingInfo rejects with a TypeError. */
    readonly rejected?: readonly string[];
    /** Content types decodingInfo throws for synchronously, like a broken polyfill. */
    readonly throwing?: readonly string[];
    /** Content types whose decodingInfo promise never settles. */
    readonly pending?: readonly string[];
  };
}

/** A timer started through `startTimer`; it fires only when the test says so. */
export interface FakeTimer {
  readonly milliseconds: number;
  readonly cancelled: boolean;
  fire(): void;
}

/** Probes answering from a fake browser, recording every decoding query and timer. */
export interface FakeProbes extends ClientProbes {
  readonly decodingQueries: DecodingConfiguration[];
  readonly timers: FakeTimer[];
}

/** `mime; codecs="…"` for every codec string. */
export function withCodecs(mimeType: string, codecStrings: readonly string[]): string[] {
  return codecStrings.map((codecString) => `${mimeType}; codecs="${codecString}"`);
}

/** Maps every type to `probably`. */
export function probably(types: readonly string[]): Record<string, PlayableAnswer> {
  return Object.fromEntries(types.map((type) => [type, "probably"]));
}

export function fakeProbes(answers: FakeBrowserAnswers): FakeProbes {
  const decodingQueries: DecodingConfiguration[] = [];
  const timers: FakeTimer[] = [];
  const knownTypes = new Set([
    ...Object.keys(answers.canPlayType),
    ...(answers.mediaSourceTypes ?? []),
  ]);
  const { mediaSourceTypes, mediaCapabilities } = answers;
  return {
    decodingQueries,
    timers,
    startTimer: (milliseconds, onTimeout) => {
      let cancelled = false;
      timers.push({
        milliseconds,
        get cancelled() {
          return cancelled;
        },
        fire: onTimeout,
      });
      return () => {
        cancelled = true;
      };
    },
    mediaElement: { canPlayType: (type) => answers.canPlayType[type] ?? "" },
    mediaSource:
      mediaSourceTypes === undefined
        ? undefined
        : { isTypeSupported: (type) => mediaSourceTypes.includes(type) },
    mediaCapabilities:
      mediaCapabilities === undefined
        ? undefined
        : {
            decodingInfo: (query) => {
              decodingQueries.push(query);
              const type = query.video?.contentType ?? query.audio?.contentType ?? "";
              if (mediaCapabilities.throwing?.includes(type) === true) {
                throw new TypeError(`broken decodingInfo for ${type}`);
              }
              if (mediaCapabilities.rejected?.includes(type) === true) {
                return Promise.reject(new TypeError(`unknown type ${type}`));
              }
              if (mediaCapabilities.pending?.includes(type) === true) {
                return new Promise<DecodingResult>(() => undefined);
              }
              const supported =
                knownTypes.has(type) && mediaCapabilities.unsupported?.includes(type) !== true;
              return Promise.resolve<DecodingResult>({ supported });
            },
          },
  };
}

const h264Main8Bit = ["avc1.42E01E", "avc1.42001E", "avc1.4D401E", "avc1.640028"];
const av1And10Bit = ["av01.0.08M.08", "av01.0.08M.10"];
const vp9And10Bit = ["vp09.00.40.08", "vp09.02.40.10"];
const desktopMp4Video = [...h264Main8Bit, ...av1And10Bit, ...vp9And10Bit];
const desktopMp4Audio = ["mp4a.40.2", "mp4a.6B", "opus", "flac"];
const webmVideo = ["vp8", ...vp9And10Bit, ...av1And10Bit];

const desktopMp4AndWebm: Readonly<Record<string, PlayableAnswer>> = {
  "video/mp4": "maybe",
  ...probably(withCodecs("video/mp4", desktopMp4Video)),
  ...probably(withCodecs("audio/mp4", desktopMp4Audio)),
  "video/webm": "maybe",
  ...probably(withCodecs("video/webm", webmVideo)),
  ...probably(withCodecs("audio/webm", ["opus", "vorbis"])),
};

const desktopMediaSourceTypes = [
  ...withCodecs("video/mp4", desktopMp4Video),
  ...withCodecs("audio/mp4", desktopMp4Audio),
];

/**
 * Chromium on desktop without platform HEVC: no High 10, no Matroska answer (ADR 0023). Like
 * Chrome/Edge 156 it answers the HLS playlist type and bare `video/ogg`, but plays no Ogg video
 * codec (Theora was removed), only Ogg audio.
 */
export const chromiumDesktop: FakeBrowserAnswers = {
  canPlayType: {
    ...desktopMp4AndWebm,
    "application/vnd.apple.mpegurl": "maybe",
    "video/ogg": "maybe",
    ...probably(withCodecs("audio/ogg", ["vorbis", "opus", "flac"])),
  },
  mediaSourceTypes: desktopMediaSourceTypes,
  mediaCapabilities: {},
};

/** Firefox on desktop: like Chromium plus Ogg with Theora, and no native HLS answer. */
export const firefoxDesktop: FakeBrowserAnswers = {
  canPlayType: {
    ...desktopMp4AndWebm,
    "video/ogg": "maybe",
    ...probably(withCodecs("video/ogg", ["theora"])),
    ...probably(withCodecs("audio/ogg", ["vorbis", "opus", "flac"])),
  },
  mediaSourceTypes: desktopMediaSourceTypes,
  mediaCapabilities: {},
};

const safariVideo = [...h264Main8Bit, "hvc1.1.6.L120.B0", "hvc1.2.4.L120.B0"];
const safariAudio = ["mp4a.40.2", "mp4a.6B", "flac", "ac-3", "ec-3"];

/**
 * Safari on iOS (best effort, OD-6): no MediaSource (only ManagedMediaSource, which the app does
 * not pass), native HLS, HEVC, QuickTime and 3GP, VP9 8-bit only.
 */
export const safariIos: FakeBrowserAnswers = {
  canPlayType: {
    "application/vnd.apple.mpegurl": "maybe",
    "video/mp4": "maybe",
    ...probably(withCodecs("video/mp4", safariVideo)),
    ...probably(withCodecs("audio/mp4", safariAudio)),
    "video/quicktime": "maybe",
    ...probably(withCodecs("video/quicktime", safariVideo)),
    "video/webm": "maybe",
    ...probably(withCodecs("video/webm", ["vp8", "vp09.00.40.08"])),
    ...probably(withCodecs("audio/webm", ["opus", "vorbis"])),
    "video/3gpp": "maybe",
    ...probably(withCodecs("video/3gpp", h264Main8Bit)),
    ...probably(withCodecs("audio/3gpp", ["mp4a.40.2"])),
  },
  mediaCapabilities: {},
};

/** Safari on macOS: the iOS answers plus MediaSource, so HLS goes through MSE. */
export const safariMacos: FakeBrowserAnswers = {
  ...safariIos,
  mediaSourceTypes: [
    ...withCodecs("video/mp4", safariVideo),
    ...withCodecs("audio/mp4", safariAudio),
  ],
};

/** An older or embedded browser: progressive H.264/AAC MP4 only, no MSE, no Media Capabilities. */
export const withoutMediaSource: FakeBrowserAnswers = {
  canPlayType: {
    "video/mp4": "maybe",
    ...probably(withCodecs("video/mp4", h264Main8Bit)),
    ...probably(withCodecs("audio/mp4", ["mp4a.40.2"])),
  },
};

import type {
  ClientProbes,
  DecodingConfiguration,
  DecodingType,
  MediaCapabilitiesProbe,
  StartTimer,
} from "./probes";

/**
 * How long one `decodingInfo` answer may take. Browsers answer in milliseconds; a promise that
 * never settles (seen with broken polyfills and extensions) must not block playback, so a late
 * answer counts as unsupported.
 */
export const decodingInfoTimeoutMilliseconds = 2000;

// The stream a decoding query describes: 1080p at 30 fps with a typical rip bit rate. Decoders
// that only fail on 4K or high frame rates still count as supported; the profile is about codecs.
const probeVideoWidth = 1920;
const probeVideoHeight = 1080;
const probeVideoBitsPerSecond = 8_000_000;
const probeVideoFramesPerSecond = 30;

export type MediaKind = "video" | "audio";

/** Whether a content type that passed the type check also decodes. */
export type DecodingCheck = (
  type: DecodingType,
  contentType: string,
  kind: MediaKind,
) => Promise<boolean>;

/**
 * Builds the decoding check for one detection run. Without Media Capabilities the type check
 * alone decides. Answers are remembered per decoding type and content type, because several
 * contexts ask the same question (MP4 and QuickTime audio, native HLS and MP4).
 */
export function createDecodingCheck(probes: ClientProbes): DecodingCheck {
  const { mediaCapabilities, startTimer } = probes;
  if (mediaCapabilities === undefined) {
    return () => Promise.resolve(true);
  }
  const answers = new Map<string, Promise<boolean>>();
  return (type, contentType, kind) => {
    const key = `${type} ${contentType}`;
    let answer = answers.get(key);
    if (answer === undefined) {
      const configuration = decodingConfiguration(type, contentType, kind);
      answer = withTimeout(askDecodingInfo(mediaCapabilities, configuration), startTimer);
      answers.set(key, answer);
    }
    return answer;
  };
}

/** decodingInfo rejects types it does not know (ADR 0023): that is an answer, not a failure. */
function askDecodingInfo(
  probe: MediaCapabilitiesProbe,
  configuration: DecodingConfiguration,
): Promise<boolean> {
  try {
    return probe.decodingInfo(configuration).then(
      ({ supported }) => supported,
      () => false,
    );
  } catch {
    return Promise.resolve(false);
  }
}

function withTimeout(answer: Promise<boolean>, startTimer: StartTimer): Promise<boolean> {
  return new Promise((resolve) => {
    const cancel = startTimer(decodingInfoTimeoutMilliseconds, () => {
      resolve(false);
    });
    void answer.then((supported) => {
      cancel();
      resolve(supported);
    });
  });
}

function decodingConfiguration(
  type: DecodingType,
  contentType: string,
  kind: MediaKind,
): DecodingConfiguration {
  if (kind === "audio") {
    return { type, audio: { contentType } };
  }
  return {
    type,
    video: {
      contentType,
      width: probeVideoWidth,
      height: probeVideoHeight,
      bitrate: probeVideoBitsPerSecond,
      framerate: probeVideoFramesPerSecond,
    },
  };
}

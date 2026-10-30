import { describe, expect, it } from "vitest";

import { streamInfo } from "./streamInfo";
import { directPlaySession, remuxSession, transcodeSession } from "./test/sessions";
import {
  keyLookup,
  streamInfoTranslationKeys,
  type StreamInfoTranslationKey,
} from "./translationKeys";

describe("streamInfoTranslationKeys", () => {
  it("lists the contract with the player's locale files", () => {
    expect(streamInfoTranslationKeys).toEqual([
      "streamInfo.mode.direct_play",
      "streamInfo.mode.remux",
      "streamInfo.mode.transcode",
      "streamInfo.mode.unknown",
      "streamInfo.reason.container_not_supported",
      "streamInfo.reason.video_codec_not_supported",
      "streamInfo.reason.video_profile_not_supported",
      "streamInfo.reason.video_bit_depth_not_supported",
      "streamInfo.reason.audio_codec_not_supported",
      "streamInfo.reason.audio_track_not_default",
      "streamInfo.reason.unknown",
      "streamInfo.warning.hdr_not_tone_mapped",
      "streamInfo.warning.subtitles_not_supported",
      "streamInfo.warning.video_downscaled",
      "streamInfo.warning.unknown",
      "streamInfo.streamKind.video",
      "streamInfo.streamKind.audio",
      "streamInfo.streamKind.subtitle",
      "streamInfo.streamKind.other",
      "streamInfo.streamKind.unknown",
      "streamInfo.action.copy",
      "streamInfo.action.transcode",
      "streamInfo.action.unknown",
      "streamInfo.encoder.software",
      "streamInfo.encoder.hardware",
      "streamInfo.bitRate.kilobits",
      "streamInfo.bitRate.megabits",
    ]);
  });

  it("has no duplicates", () => {
    expect(new Set(streamInfoTranslationKeys).size).toBe(streamInfoTranslationKeys.length);
  });

  it("contains every key the view model of the example sessions uses", () => {
    const listed: ReadonlySet<string> = new Set(streamInfoTranslationKeys);
    const used = [directPlaySession, remuxSession, transcodeSession].flatMap(usedKeys);

    expect(used.filter((key) => !listed.has(key))).toEqual([]);
  });
});

describe("keyLookup", () => {
  const lookup = keyLookup("action", ["copy", "transcode"]);

  it.each([
    ["copy", "streamInfo.action.copy"],
    ["transcode", "streamInfo.action.transcode"],
    ["passthrough", "streamInfo.action.unknown"],
    ["", "streamInfo.action.unknown"],
    ["__proto__", "streamInfo.action.unknown"],
  ])("maps %j to %s", (value, expected) => {
    expect(lookup(value)).toBe(expected);
  });
});

function usedKeys(session: typeof transcodeSession): StreamInfoTranslationKey[] {
  const info = streamInfo(session);
  return [
    info.mode.key,
    ...info.reasons.map((reason) => reason.key),
    ...info.warnings.map((warning) => warning.key),
    ...info.streams.flatMap((line) => [line.kindKey, line.actionKey]),
    ...(info.encoder === undefined ? [] : [info.encoder.key]),
    ...(info.sourceBitRate === undefined ? [] : [info.sourceBitRate.key]),
  ];
}

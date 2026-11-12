import {
  streamInfo,
  type AudioTrack,
  type PlaybackSession,
  type SubtitleTrackSummary,
} from "@mivo/player-core";
import { cleanup, render, screen, within } from "@testing-library/react";
import { createRef } from "react";
import { I18nextProvider } from "react-i18next";
import { afterEach, describe, expect, it } from "vitest";

import { createAppI18n } from "../../../app/i18n";
import type { NetworkStats } from "../hooks/useNetworkStats";
import { directPlaySession, transcodeSession } from "../test/playerFixtures";
import { StreamInfoOverlay } from "./StreamInfoOverlay";

afterEach(cleanup);

const aacTrack: AudioTrack = {
  index: 3,
  codec: "aac",
  channels: 2,
  default: false,
  commentary: false,
};
const surroundTrack: AudioTrack = {
  index: 4,
  codec: "eac3",
  channels: 6,
  default: false,
  commentary: false,
};

function renderOverlay(
  networkStats: NetworkStats,
  language = "en",
  alternativeAudio: readonly AudioTrack[] = [aacTrack],
) {
  render(
    <I18nextProvider i18n={createAppI18n([language])}>
      <StreamInfoOverlay
        info={streamInfo(transcodeSession)}
        networkStats={networkStats}
        alternativeAudio={alternativeAudio}
        regionRef={createRef<HTMLElement>()}
        headingRef={createRef<HTMLHeadingElement>()}
        onClose={() => undefined}
      />
    </I18nextProvider>,
  );
  return within(screen.getByRole("region"));
}

function renderSession(
  session: PlaybackSession,
  subtitleTracks: readonly SubtitleTrackSummary[] = [],
  language = "en",
) {
  render(
    <I18nextProvider i18n={createAppI18n([language])}>
      <StreamInfoOverlay
        info={streamInfo(session, subtitleTracks)}
        networkStats={{ bufferAheadSeconds: undefined, throughputBytesPerSecond: undefined }}
        alternativeAudio={[]}
        regionRef={createRef<HTMLElement>()}
        headingRef={createRef<HTMLHeadingElement>()}
        onClose={() => undefined}
      />
    </I18nextProvider>,
  );
  return within(screen.getByRole("region"));
}

function valueOf(overlay: ReturnType<typeof renderSession>, label: string): string {
  return overlay.getByText(label).nextElementSibling?.textContent ?? "";
}

describe("StreamInfoOverlay transcode details (PL-41)", () => {
  it("shows output bit rate, hardware encoder, subtitle mode with track and speed", () => {
    const overlay = renderSession(
      {
        ...transcodeSession,
        outputBitRate: 8_000_000,
        transcodeSpeed: 2.4,
        subtitle: { mode: "burn_in", trackId: "s:3" },
        streams: transcodeSession.streams.map((stream) =>
          stream.kind === "video" ? { ...stream, encoder: "h264_vaapi", hardware: true } : stream,
        ),
      },
      [{ id: "s:3", language: "eng", format: "hdmv_pgs_subtitle" }],
    );

    expect(valueOf(overlay, "Output bit rate")).toBe("8 Mbit/s");
    expect(valueOf(overlay, "Encoder")).toBe("Hardware (h264_vaapi)");
    expect(valueOf(overlay, "Subtitles")).toBe(
      "Burned into the video – the video is converted · eng · PGS",
    );
    expect(valueOf(overlay, "Speed")).toBe("2.4× real time");
    expect(overlay.queryByText("too slow – stuttering likely")).toBeNull();
  });

  it("warns in the transcode colour when the speed is below real time", () => {
    const overlay = renderSession({ ...transcodeSession, transcodeSpeed: 0.8 });

    const warning = overlay.getByText("too slow – stuttering likely");
    expect(warning.closest("dd")?.className).toContain("text-transcode");
    expect(valueOf(overlay, "Speed")).toContain("0.8× real time");
  });

  it("says unknown for a speed FFmpeg has not reported and leaves it out for direct play", () => {
    expect(valueOf(renderSession(transcodeSession), "Speed")).toBe("unknown");
    cleanup();

    expect(renderSession(directPlaySession).queryByText("Speed")).toBeNull();
  });

  it("speaks German", () => {
    const overlay = renderSession(
      { ...transcodeSession, transcodeSpeed: 0.5, subtitle: { mode: "sidecar", trackId: "s:1" } },
      [],
      "de",
    );

    expect(valueOf(overlay, "Bitrate der Ausgabe")).toBe("unbekannt");
    expect(valueOf(overlay, "Untertitel")).toBe("Im Player gerendert (WebVTT)");
    expect(overlay.getByText("zu langsam – Ruckeln wahrscheinlich")).toBeTruthy();
  });
});

describe("StreamInfoOverlay extras", () => {
  it("shows buffer ahead in seconds and throughput in megabytes per second", () => {
    const overlay = renderOverlay({
      bufferAheadSeconds: 29.6,
      throughputBytesPerSecond: 2_450_000,
    });

    expect(overlay.getByText("Buffer ahead").nextElementSibling?.textContent).toBe("30 s");
    expect(overlay.getByText("Network").nextElementSibling?.textContent).toBe("2.5 MB/s");
  });

  it("formats the numbers for the language", () => {
    const overlay = renderOverlay(
      { bufferAheadSeconds: 5, throughputBytesPerSecond: 2_450_000 },
      "de",
    );

    expect(overlay.getByText("Netzwerk").nextElementSibling?.textContent).toBe("2,5 MB/s");
  });

  it("says unknown for values not measured yet", () => {
    const overlay = renderOverlay({
      bufferAheadSeconds: undefined,
      throughputBytesPerSecond: undefined,
    });

    expect(overlay.getByText("Buffer ahead").nextElementSibling?.textContent).toBe("unknown");
    expect(overlay.getByText("Network").nextElementSibling?.textContent).toBe("unknown");
  });

  it("shows an action hint only for reasons that have one", () => {
    const overlay = renderOverlay({ bufferAheadSeconds: 0, throughputBytesPerSecond: undefined });

    expect(overlay.getAllByText("Choose an AAC track to avoid converting.")).toHaveLength(1);
    const container = overlay.getByText(/^MKV not supported/);
    expect(container.textContent).toBe("MKV not supported → MP4");
  });

  it.each([
    { language: "en", hint: "Choose an AAC track to avoid converting." },
    { language: "de", hint: "Wähle eine AAC-Spur, um die Umwandlung zu vermeiden." },
  ])("shows the AAC hint in $language when an AAC track exists", ({ language, hint }) => {
    const overlay = renderOverlay({ bufferAheadSeconds: 0, throughputBytesPerSecond: 1 }, language);

    expect(overlay.getAllByText(hint)).toHaveLength(1);
  });

  it.each(["en", "de"])("hides the AAC hint in %s without an AAC alternative", (language) => {
    const overlay = renderOverlay(
      { bufferAheadSeconds: 0, throughputBytesPerSecond: 1 },
      language,
      [surroundTrack],
    );

    expect(overlay.queryByText(/AAC/, { selector: "span.block" })).toBeNull();
  });
});

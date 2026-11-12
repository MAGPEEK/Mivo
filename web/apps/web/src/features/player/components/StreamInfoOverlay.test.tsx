import { streamInfo } from "@mivo/player-core";
import { cleanup, render, screen, within } from "@testing-library/react";
import { createRef } from "react";
import { I18nextProvider } from "react-i18next";
import { afterEach, describe, expect, it } from "vitest";

import { createAppI18n } from "../../../app/i18n";
import type { NetworkStats } from "../hooks/useNetworkStats";
import { transcodeSession } from "../test/playerFixtures";
import { StreamInfoOverlay } from "./StreamInfoOverlay";

afterEach(cleanup);

function renderOverlay(networkStats: NetworkStats, language = "en") {
  render(
    <I18nextProvider i18n={createAppI18n([language])}>
      <StreamInfoOverlay
        info={streamInfo(transcodeSession)}
        networkStats={networkStats}
        regionRef={createRef<HTMLElement>()}
        headingRef={createRef<HTMLHeadingElement>()}
        onClose={() => undefined}
      />
    </I18nextProvider>,
  );
  return within(screen.getByRole("region"));
}

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
});

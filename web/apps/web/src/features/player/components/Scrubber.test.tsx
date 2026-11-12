import type { TrickplayLayout } from "@mivo/player-core";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nextProvider } from "react-i18next";
import { afterEach, describe, expect, it, vi } from "vitest";

import { createAppI18n } from "../../../app/i18n";
import { ApiClientProvider } from "../../../shared/api";
import { blockingAxeViolations } from "../../../test/axe";
import { createFakeApiClient } from "../../../test/fakeApi";
import type { TrickplayApi } from "../api/trickplayApi";
import type { TrickplayServices } from "../hooks/useTrickplay";
import type { SheetLoader } from "../lib/trickplaySheets";
import { Scrubber } from "./Scrubber";

afterEach(cleanup);

const duration = 1000;
const trackBox = { left: 0, width: 500 } as DOMRect;

const layout: TrickplayLayout = {
  tileWidth: 160,
  tileHeight: 90,
  columns: 10,
  rows: 10,
  intervalSeconds: 10,
  sheetUrls: ["/api/v1/media-files/file-1/trickplay/0.jpg"],
};

function servicesFor(manifest: TrickplayLayout | undefined, loadSheetResult = true) {
  const getManifest = vi.fn<TrickplayApi["getManifest"]>(() => Promise.resolve(manifest));
  const requestGeneration = vi.fn<TrickplayApi["requestGeneration"]>(() => Promise.resolve());
  const loadSheet = vi.fn<SheetLoader>(() => ({
    result: Promise.resolve(loadSheetResult),
    cancel: vi.fn(),
  }));
  const services: TrickplayServices = {
    api: { getManifest, requestGeneration },
    loadSheet,
    waits: { delay: () => new Promise(() => undefined), visible: () => Promise.resolve() },
  };
  return { services, getManifest, loadSheet };
}

function renderScrubber(services: TrickplayServices, position = 100, onSeek = vi.fn()) {
  const view = render(
    <I18nextProvider i18n={createAppI18n(["en"])}>
      <ApiClientProvider client={createFakeApiClient()}>
        <Scrubber
          position={position}
          duration={duration}
          producedRanges={[]}
          onSeek={onSeek}
          fileId="file-1"
          trickplayServices={services}
        />
      </ApiClientProvider>
    </I18nextProvider>,
  );
  const slider = screen.getByRole("slider");
  const wrapper = slider.parentElement;
  if (wrapper === null) {
    throw new Error("the slider has no wrapper");
  }
  wrapper.getBoundingClientRect = () => trackBox;
  return { ...view, slider, wrapper, onSeek };
}

describe("Scrubber preview", () => {
  it("shows the tile cropped by CSS and the time while hovering", async () => {
    const { services, getManifest } = servicesFor(layout);
    const { wrapper } = renderScrubber(services);
    await waitFor(() => {
      expect(getManifest).toHaveBeenCalledTimes(1);
    });

    // 250 of 500 px is 500 s: tile 50 = sheet 0, column 0, row 5.
    fireEvent.pointerMove(wrapper, { clientX: 250, pointerType: "mouse" });

    const tile = await screen.findByTestId("seek-preview-tile");
    expect(tile.style.backgroundImage).toContain("/api/v1/media-files/file-1/trickplay/0.jpg");
    expect(tile.style.backgroundPosition).toBe("0px -450px");
    expect(screen.getByTestId("seek-preview").textContent).toBe("8:20");
    expect(screen.getByTestId("seek-preview").getAttribute("aria-hidden")).toBe("true");
  });

  it("shows the time only while the file has no trickplay, and hides the bubble on leave", () => {
    const { services } = servicesFor(undefined);
    const { wrapper } = renderScrubber(services);

    fireEvent.pointerMove(wrapper, { clientX: 50, pointerType: "mouse" });
    expect(screen.getByTestId("seek-preview").textContent).toBe("1:40");
    expect(screen.queryByTestId("seek-preview-tile")).toBeNull();

    fireEvent.pointerLeave(wrapper);
    expect(screen.queryByTestId("seek-preview")).toBeNull();
  });

  it("does not load a sheet before the viewer hovers", async () => {
    const { services, getManifest, loadSheet } = servicesFor(layout);
    renderScrubber(services);

    await waitFor(() => {
      expect(getManifest).toHaveBeenCalledTimes(1);
    });
    expect(loadSheet).not.toHaveBeenCalled();
  });

  it("shows the same preview for the arrow keys", async () => {
    const { services, getManifest } = servicesFor(layout);
    const { slider, onSeek } = renderScrubber(services, 100);
    await waitFor(() => {
      expect(getManifest).toHaveBeenCalledTimes(1);
    });

    fireEvent.keyDown(slider, { key: "ArrowRight" });

    const tile = await screen.findByTestId("seek-preview-tile");
    // 110 s is tile 11: column 1, row 1.
    expect(tile.style.backgroundPosition).toBe("-160px -90px");
    expect(screen.getByTestId("seek-preview").textContent).toBe("1:50");
    expect(onSeek).toHaveBeenCalledExactlyOnceWith(110);

    fireEvent.blur(slider);
    expect(screen.queryByTestId("seek-preview")).toBeNull();
  });

  it("follows a drag, which is how a finger scrubs", async () => {
    const { services, getManifest } = servicesFor(layout);
    const { slider } = renderScrubber(services);
    await waitFor(() => {
      expect(getManifest).toHaveBeenCalledTimes(1);
    });

    fireEvent.pointerDown(slider);
    fireEvent.change(slider, { target: { value: "600" } });

    const tile = await screen.findByTestId("seek-preview-tile");
    expect(tile.style.backgroundPosition).toBe("0px -540px");
    fireEvent.pointerUp(window);
    expect(screen.queryByTestId("seek-preview")).toBeNull();
  });

  it("ignores a touch that only moves over the bar", () => {
    const { services } = servicesFor(layout);
    const { wrapper } = renderScrubber(services);

    fireEvent.pointerMove(wrapper, { clientX: 250, pointerType: "touch" });

    expect(screen.queryByTestId("seek-preview")).toBeNull();
  });

  it("refuses a sheet on another origin and shows the time only", async () => {
    const foreign = { ...layout, sheetUrls: ["https://evil.test/0.jpg"] };
    const { services, getManifest, loadSheet } = servicesFor(foreign);
    const { wrapper } = renderScrubber(services);
    await waitFor(() => {
      expect(getManifest).toHaveBeenCalledTimes(1);
    });

    fireEvent.pointerMove(wrapper, { clientX: 250, pointerType: "mouse" });

    expect(screen.queryByTestId("seek-preview")).not.toBeNull();
    expect(screen.queryByTestId("seek-preview-tile")).toBeNull();
    expect(loadSheet).not.toHaveBeenCalled();
  });

  it("shows the time only when the sheet fails to load", async () => {
    const { services, getManifest, loadSheet } = servicesFor(layout, false);
    const { wrapper } = renderScrubber(services);
    await waitFor(() => {
      expect(getManifest).toHaveBeenCalledTimes(1);
    });

    fireEvent.pointerMove(wrapper, { clientX: 250, pointerType: "mouse" });
    await waitFor(() => {
      expect(loadSheet).toHaveBeenCalledTimes(1);
    });

    expect(screen.queryByTestId("seek-preview-tile")).toBeNull();
    expect(screen.getByTestId("seek-preview").textContent).toBe("8:20");
  });

  it("has no blocking accessibility violations with the preview open", async () => {
    const { services, getManifest } = servicesFor(layout);
    const { wrapper, container } = renderScrubber(services);
    await waitFor(() => {
      expect(getManifest).toHaveBeenCalledTimes(1);
    });
    fireEvent.pointerMove(wrapper, { clientX: 250, pointerType: "mouse" });
    await screen.findByTestId("seek-preview-tile");

    expect(await blockingAxeViolations(container)).toEqual([]);
  });
});

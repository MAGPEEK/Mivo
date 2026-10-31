import type { DecodingConfiguration } from "@mivo/player-core";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  browserName,
  browserProbes,
  browserStartTimer,
  currentBrowser,
  detectBrowserProfile,
  unknownBrowserName,
  type BrowserEnvironment,
} from "./browserProfile";

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("browserName", () => {
  it.each([
    {
      userAgent:
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36 Edg/156.0.0.0",
      expected: "Edge 156",
    },
    {
      userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:159.0) Gecko/20100101 Firefox/159.0",
      expected: "Firefox 159",
    },
    {
      userAgent:
        "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36",
      expected: "Chrome 156",
    },
    {
      userAgent:
        "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1",
      expected: "Safari 18",
    },
    {
      userAgent:
        "Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 Chrome/150.0.0.0 Safari/537.36 OPR/120.0",
      expected: "Opera 120",
    },
    { userAgent: "Mozilla/5.0 Chrome/", expected: "Chrome" },
    { userAgent: "curl/8.0", expected: unknownBrowserName },
  ])("names $expected", ({ userAgent, expected }) => {
    expect(browserName(userAgent)).toBe(expected);
  });
});

describe("browserStartTimer", () => {
  it("fires after the delay unless cancelled", () => {
    vi.useFakeTimers();
    const fired = vi.fn();
    const cancelled = vi.fn();

    browserStartTimer(100, fired);
    browserStartTimer(100, cancelled)();
    vi.advanceTimersByTime(100);

    expect(fired).toHaveBeenCalledOnce();
    expect(cancelled).not.toHaveBeenCalled();
  });
});

function fakeBrowser(overrides: Partial<BrowserEnvironment> = {}): BrowserEnvironment {
  return {
    createVideoElement: () => ({
      canPlayType: (type: string) => (type.startsWith("video/mp4") ? "probably" : ""),
    }),
    mediaSource: { isTypeSupported: (type: string) => type.startsWith("video/mp4") },
    mediaCapabilities: undefined,
    startTimer: browserStartTimer,
    userAgent: "Mozilla/5.0 Firefox/159.0",
    ...overrides,
  };
}

describe("detectBrowserProfile", () => {
  it("detects the profile from the browser's probes, named after the browser", async () => {
    const profile = await detectBrowserProfile(fakeBrowser());

    expect(profile.name).toBe("Firefox 159");
    expect(profile.hls.delivery).toBe("mse");
    expect(profile.directPlay.map((entry) => entry.container)).toEqual(["mp4"]);
  });

  it("passes Media Capabilities through to player-core", async () => {
    const decodingInfo = vi.fn((configuration: DecodingConfiguration) =>
      Promise.resolve({ supported: configuration.video?.contentType.includes("avc1") ?? true }),
    );

    const profile = await detectBrowserProfile(
      fakeBrowser({ mediaCapabilities: { decodingInfo } }),
    );

    expect(decodingInfo).toHaveBeenCalled();
    expect(profile.hls.videoCodecs).toEqual(["h264"]);
  });

  it("reads the current browser by default (jsdom plays nothing)", async () => {
    const profile = await detectBrowserProfile();

    expect(profile).toMatchObject({ directPlay: [], hls: { delivery: "none" } });
  });
});

describe("currentBrowser", () => {
  it("uses MSE and Media Capabilities when the browser has them", () => {
    const mediaSource = { isTypeSupported: () => true };
    const mediaCapabilities = { decodingInfo: () => Promise.resolve({ supported: true }) };
    vi.stubGlobal("MediaSource", mediaSource);
    vi.stubGlobal("navigator", { userAgent: "Test/1.0", mediaCapabilities });

    const browser = currentBrowser();

    expect(browser).toMatchObject({ mediaSource, mediaCapabilities, userAgent: "Test/1.0" });
    expect(browserProbes(browser).mediaElement).toBeInstanceOf(HTMLVideoElement);
  });

  it("leaves MSE and Media Capabilities out when the browser lacks them", () => {
    expect(currentBrowser()).toMatchObject({
      mediaSource: undefined,
      mediaCapabilities: undefined,
    });
  });
});

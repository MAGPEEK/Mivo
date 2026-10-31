// Feeds player-core's profile detection with the real browser probes (ADR 0023). The globals are
// read in one place, `currentBrowser`, so the rest stays testable with fakes.

import {
  detectClientProfile,
  type ClientProbes,
  type ClientProfile,
  type MediaCapabilitiesProbe,
  type MediaElementProbe,
  type MediaSourceProbe,
  type StartTimer,
} from "@mivo/player-core";

/** The browser APIs a profile is detected from. */
export interface BrowserEnvironment {
  /** Creates the element whose `canPlayType` answers progressive and native HLS playback. */
  readonly createVideoElement: () => MediaElementProbe;
  readonly mediaSource: MediaSourceProbe | undefined;
  readonly mediaCapabilities: MediaCapabilitiesProbe | undefined;
  readonly startTimer: StartTimer;
  readonly userAgent: string;
}

/** Starts a one-shot timer with the browser's `setTimeout`; the result cancels it. */
export const browserStartTimer: StartTimer = (milliseconds, onTimeout) => {
  const id = setTimeout(onTimeout, milliseconds);
  return () => {
    clearTimeout(id);
  };
};

/** The APIs of the browser this code runs in; MSE and Media Capabilities may be missing. */
export function currentBrowser(): BrowserEnvironment {
  return {
    createVideoElement: () => document.createElement("video"),
    mediaSource: "MediaSource" in window ? window.MediaSource : undefined,
    mediaCapabilities: "mediaCapabilities" in navigator ? navigator.mediaCapabilities : undefined,
    startTimer: browserStartTimer,
    userAgent: navigator.userAgent,
  };
}

/** Browsers by user-agent token, most specific first (Edge and Opera also send "Chrome/"). */
const browserTokens: readonly (readonly [token: string, name: string])[] = [
  ["Edg/", "Edge"],
  ["OPR/", "Opera"],
  ["Firefox/", "Firefox"],
  ["Chrome/", "Chrome"],
  ["Version/", "Safari"],
];

/** Name of a browser the user-agent string does not reveal. */
export const unknownBrowserName = "Web browser";

/**
 * A short client name such as "Firefox 159" for server logs and the stream-info overlay. It
 * is free text (ADR 0023), so the user agent is only shortened, never interpreted further.
 */
export function browserName(userAgent: string): string {
  for (const [token, name] of browserTokens) {
    const start = userAgent.indexOf(token);
    if (start !== -1) {
      const [majorVersion = ""] = userAgent.slice(start + token.length).split(/[.\s]/, 1);
      return majorVersion === "" ? name : `${name} ${majorVersion}`;
    }
  }
  return unknownBrowserName;
}

/** The probes player-core needs, from the given browser. */
export function browserProbes(browser: BrowserEnvironment): ClientProbes {
  return {
    mediaElement: browser.createVideoElement(),
    mediaSource: browser.mediaSource,
    mediaCapabilities: browser.mediaCapabilities,
    startTimer: browser.startTimer,
  };
}

/** Detects what the given browser plays (defaults to the current one). */
export function detectBrowserProfile(
  browser: BrowserEnvironment = currentBrowser(),
): Promise<ClientProfile> {
  return detectClientProfile(browserProbes(browser), { name: browserName(browser.userAgent) });
}

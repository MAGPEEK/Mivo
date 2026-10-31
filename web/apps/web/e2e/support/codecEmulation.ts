// Emulates a browser that lacks some formats by making its media probes deny them. Installed
// Chromium builds differ in what they decode (Playwright's Chromium plays H.264 on Windows,
// Chrome 156 plays Matroska), so a spec that needs a gap creates it instead of hoping for it.
import type { Page } from "@playwright/test";

/** Codec strings of the codecs a Chromium without proprietary codecs lacks (RFC 6381). */
export const proprietaryCodecs = "avc1|avc3|hvc1|hev1|mp4a\\.40";

/** The Matroska MIME types, which browsers before Chrome's Matroska support do not play. */
export const matroskaTypes = "x-matroska";

/**
 * Runs in the browser before the app: `canPlayType`, `MediaSource.isTypeSupported` and
 * `mediaCapabilities.decodingInfo` deny every type matching `pattern`, so the player's profile
 * detection leaves those formats out.
 */
function denyMediaTypes(pattern: string): void {
  const denied = new RegExp(pattern, "i");
  // eslint-disable-next-line @typescript-eslint/unbound-method -- only called with .call(this)
  const canPlayType = HTMLMediaElement.prototype.canPlayType;
  HTMLMediaElement.prototype.canPlayType = function (this: HTMLMediaElement, type: string) {
    return denied.test(type) ? "" : canPlayType.call(this, type);
  };
  const isTypeSupported = MediaSource.isTypeSupported.bind(MediaSource);
  MediaSource.isTypeSupported = (type) => !denied.test(type) && isTypeSupported(type);
  const capabilities = navigator.mediaCapabilities;
  const decodingInfo = capabilities.decodingInfo.bind(capabilities);
  capabilities.decodingInfo = (configuration) => {
    const types = [configuration.video?.contentType, configuration.audio?.contentType];
    if (types.some((type) => type !== undefined && denied.test(type))) {
      return Promise.resolve({
        supported: false,
        smooth: false,
        powerEfficient: false,
        keySystemAccess: null,
      });
    }
    return decodingInfo(configuration);
  };
}

/**
 * Makes every document `page` loads from now on deny the media types matching `pattern` (a
 * regular expression source such as {@link proprietaryCodecs}).
 */
export async function denyMediaTypesFromNowOn(page: Page, pattern: string): Promise<void> {
  await page.addInitScript(denyMediaTypes, pattern);
}

/** Whether the browser of `page` itself plays `type`, i.e. before any emulation. */
export function browserPlays(page: Page, type: string): Promise<boolean> {
  return page.evaluate(
    (mimeType) => document.createElement("video").canPlayType(mimeType) !== "",
    type,
  );
}

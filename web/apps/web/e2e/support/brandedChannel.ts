// The branded browser of the `branded` project (ADR 0025, OD-7): an installed Edge or Chrome
// with the proprietary codecs a real user's browser has. Playwright does not install these; a
// machine without the channel skips the project with a visible reason instead of failing.
import { productId } from "@mivo/ui-tokens";
import { existsSync } from "node:fs";
import { join } from "node:path";

/** Environment variable that overrides the channel, e.g. `chrome` or `msedge-beta`. */
export const brandedChannelVariable = `${productId.toUpperCase()}_E2E_BRANDED_CHANNEL`;

/** The branded channel of this run and whether it is installed. */
export interface BrandedChannel {
  /** Playwright channel name. */
  readonly channel: string;
  /** False when none of the channel's known install locations exists. */
  readonly installed: boolean;
  /** Why the branded specs are skipped; empty when the channel is installed. */
  readonly skipReason: string;
}

type InstallLocations = (environment: NodeJS.ProcessEnv) => readonly string[];

function windowsLocations(relativePath: string): InstallLocations {
  return (environment) =>
    [environment["PROGRAMFILES"], environment["PROGRAMFILES(X86)"], environment["LOCALAPPDATA"]]
      .filter((base): base is string => base !== undefined && base !== "")
      .map((base) => join(base, relativePath));
}

// Where Playwright looks for the stable channels (its registry); other channels such as betas
// are trusted to exist, and Playwright reports it clearly when they do not.
const knownLocations: Readonly<Record<string, Partial<Record<NodeJS.Platform, InstallLocations>>>> =
  {
    msedge: {
      win32: windowsLocations(join("Microsoft", "Edge", "Application", "msedge.exe")),
      darwin: () => ["/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"],
      linux: () => ["/opt/microsoft/msedge/msedge"],
    },
    chrome: {
      win32: windowsLocations(join("Google", "Chrome", "Application", "chrome.exe")),
      darwin: () => ["/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"],
      linux: () => ["/opt/google/chrome/chrome"],
    },
  };

function isInstalled(channel: string, environment: NodeJS.ProcessEnv): boolean {
  const locations = knownLocations[channel]?.[process.platform];
  return locations === undefined || locations(environment).some((path) => existsSync(path));
}

/**
 * The branded channel: the override variable, else Edge on Windows (always installed there)
 * and Chrome elsewhere.
 */
export function brandedChannel(environment: NodeJS.ProcessEnv = process.env): BrandedChannel {
  const override = environment[brandedChannelVariable]?.trim() ?? "";
  const fallback = process.platform === "win32" ? "msedge" : "chrome";
  const channel = override === "" ? fallback : override;
  const installed = isInstalled(channel, environment);
  return {
    channel,
    installed,
    skipReason: installed
      ? ""
      : `The browser channel "${channel}" is not installed. Install it or set ` +
        `${brandedChannelVariable} to an installed channel (e.g. chrome or msedge).`,
  };
}

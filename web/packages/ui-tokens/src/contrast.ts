// WCAG 2.x contrast ratio, see https://www.w3.org/TR/WCAG21/#dfn-contrast-ratio.

const shortHexPattern = /^#([0-9a-f])([0-9a-f])([0-9a-f])$/i;
const longHexPattern = /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i;
const hexRadix = 16;
const maxChannelValue = 255;

// sRGB transfer function constants from the WCAG definition of relative luminance.
const linearThreshold = 0.04045;
const linearDivisor = 12.92;
const gammaOffset = 0.055;
const gammaDivisor = 1.055;
const gammaExponent = 2.4;

// Luminance weights of the red, green and blue channels.
const redWeight = 0.2126;
const greenWeight = 0.7152;
const blueWeight = 0.0722;

// Flare term added to both luminances, which also keeps black on black at 1 instead of 0 / 0.
const flare = 0.05;

/** Thrown when a value is not a `#rgb` or `#rrggbb` colour. */
export class InvalidColorError extends Error {
  override readonly name = "InvalidColorError";

  /** The rejected input. */
  readonly value: string;

  constructor(value: string) {
    super(`Invalid colour "${value}": expected #rgb or #rrggbb`);
    this.value = value;
  }
}

function parseHexChannels(color: string): readonly [string, string, string] {
  const short = shortHexPattern.exec(color);
  if (short) {
    const [, red = "", green = "", blue = ""] = short;
    return [red + red, green + green, blue + blue];
  }
  const long = longHexPattern.exec(color);
  if (long) {
    const [, red = "", green = "", blue = ""] = long;
    return [red, green, blue];
  }
  throw new InvalidColorError(color);
}

function linearize(channelHex: string): number {
  const channel = Number.parseInt(channelHex, hexRadix) / maxChannelValue;
  return channel <= linearThreshold
    ? channel / linearDivisor
    : ((channel + gammaOffset) / gammaDivisor) ** gammaExponent;
}

/**
 * Relative luminance of a `#rgb` or `#rrggbb` colour, from 0 (black) to 1 (white).
 * @throws {InvalidColorError} if the colour cannot be parsed.
 */
export function relativeLuminance(color: string): number {
  const [red, green, blue] = parseHexChannels(color);
  return redWeight * linearize(red) + greenWeight * linearize(green) + blueWeight * linearize(blue);
}

/**
 * WCAG 2.x contrast ratio between two colours, from 1 (identical) to 21 (black on white).
 * The result does not depend on which colour is the foreground.
 * @throws {InvalidColorError} if either colour cannot be parsed.
 */
export function contrastRatio(foreground: string, background: string): number {
  const foregroundLuminance = relativeLuminance(foreground);
  const backgroundLuminance = relativeLuminance(background);
  const lighter = Math.max(foregroundLuminance, backgroundLuminance);
  const darker = Math.min(foregroundLuminance, backgroundLuminance);
  return (lighter + flare) / (darker + flare);
}

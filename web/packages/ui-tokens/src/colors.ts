/** A colour written as `#rrggbb`. Tokens use the six-digit form so they compare exactly. */
export type HexColor = `#${string}`;

/**
 * Colour tokens of the dark-first coral theme (ADR 0011). `theme.css` declares the same values
 * for Tailwind; a drift test keeps both in sync.
 */
export const colors = {
  /** Page background. */
  background: "#0E1014",
  /** Cards, sidebar and top bar. */
  surface: "#181B21",
  /** Elements above a surface: menus, dialogs, hovered cards. */
  surfaceRaised: "#22262E",
  /** Hairlines and borders between areas. */
  divider: "#2C313A",
  /** Primary text. */
  text: "#E8EAED",
  /** Secondary text such as metadata and captions. */
  textMuted: "#9AA0A6",
  /** Brand accent (coral): primary actions, focus ring, progress. */
  accent: "#FF6B57",
  /** Accent while hovered or pressed. */
  accentHover: "#FF8572",
  /** Text and icons placed on the accent colour. */
  onAccent: "#3A0E07",
  /** Playback status: the client plays the original file. */
  directPlay: "#3DD68C",
  /** Playback status: the server remuxes into another container. */
  directStream: "#4C9AFF",
  /** Playback status: the server transcodes audio or video. */
  transcode: "#F5B544",
} as const satisfies Readonly<Record<string, HexColor>>;

/** Name of a colour token. */
export type ColorToken = keyof typeof colors;

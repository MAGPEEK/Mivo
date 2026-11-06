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
  /** Backdrop behind modal dialogs; used with an opacity such as `bg-scrim/60`. */
  scrim: "#000000",
  /** Playback status: the client plays the original file. */
  directPlay: "#3DD68C",
  /** Playback status: the server remuxes into another container. */
  directStream: "#4C9AFF",
  /** Playback status: the server transcodes audio or video. */
  transcode: "#F5B544",
  /** Profile avatars (AvatarColor in api/openapi.yaml); the initial is written in `background`. */
  avatarCoral: "#FF8572",
  avatarAmber: "#F5B544",
  avatarGreen: "#3DD68C",
  avatarTeal: "#2EC4B6",
  avatarBlue: "#4C9AFF",
  avatarViolet: "#A78BFA",
  avatarPink: "#F47FB5",
  avatarSlate: "#8E9AAF",
} as const satisfies Readonly<Record<string, HexColor>>;

/** Names of the profile avatar colours, the values of the API's `AvatarColor`. */
export const avatarColorNames = [
  "coral",
  "amber",
  "green",
  "teal",
  "blue",
  "violet",
  "pink",
  "slate",
] as const;

/** One profile avatar colour name. */
export type AvatarColorName = (typeof avatarColorNames)[number];

/** Name of a colour token. */
export type ColorToken = keyof typeof colors;

/**
 * Opacity of a `background` scrim that carries text over an arbitrary image, such as the
 * Home hero backdrop (`from-background/90`). It keeps `text` and `textMuted` at 4.5:1 or more
 * even over a white image (colors.test.ts).
 */
export const textScrimOpacity = 0.9;

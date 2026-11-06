import type { AvatarColorName } from "@mivo/ui-tokens";

// Literal class names, so Tailwind finds them in the source.
const backgroundClasses: Readonly<Record<AvatarColorName, string>> = {
  coral: "bg-avatar-coral",
  amber: "bg-avatar-amber",
  green: "bg-avatar-green",
  teal: "bg-avatar-teal",
  blue: "bg-avatar-blue",
  violet: "bg-avatar-violet",
  pink: "bg-avatar-pink",
  slate: "bg-avatar-slate",
};

function isAvatarColorName(color: string): color is AvatarColorName {
  return Object.hasOwn(backgroundClasses, color);
}

/**
 * The background class of an avatar colour from the API. The API may add colours later
 * (`AvatarColor` asks clients to tolerate new values); those show in slate.
 */
export function avatarBackgroundClass(color: string): string {
  return isAvatarColorName(color) ? backgroundClasses[color] : backgroundClasses.slate;
}

/** The letter an avatar shows: the first character of the name, upper case. */
export function avatarInitial(name: string): string {
  const first = Array.from(name.trim())[0];
  return first === undefined ? "?" : first.toLocaleUpperCase();
}

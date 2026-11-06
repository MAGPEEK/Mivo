import type { ReactNode } from "react";

import { avatarBackgroundClass, avatarInitial } from "../lib/avatar";

/** Props of {@link Avatar}. */
export interface AvatarProps {
  readonly name: string;
  readonly color: string;
  readonly size: "small" | "medium" | "large";
  /** A corner marker such as the PIN lock. */
  readonly marker?: ReactNode;
}

const sizeClasses = {
  small: "size-8 text-sm",
  medium: "size-10 text-lg",
  large: "size-24 text-4xl",
} as const;

/** The round initial on the profile's token colour. Decorative: the name is always next to it. */
export function Avatar({ name, color, size, marker }: AvatarProps) {
  return (
    <span
      className={`relative grid shrink-0 place-items-center rounded-full font-extrabold text-background ${sizeClasses[size]} ${avatarBackgroundClass(color)}`}
    >
      <span aria-hidden="true">{avatarInitial(name)}</span>
      {marker}
    </span>
  );
}

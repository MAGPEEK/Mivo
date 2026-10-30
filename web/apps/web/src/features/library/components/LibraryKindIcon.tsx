import type { LibraryKind } from "@mivo/api-client";
import type { ReactNode } from "react";

import { LibraryIcon } from "../../../shared/ui";

// Same frame as the shared icons: 24-unit grid, line drawing in the current text colour,
// decorative (the link next to it carries the library name).
const viewBox = "0 0 24 24";
const strokeWidth = 1.75;

interface KindIconProps {
  /** Exposed as `data-icon`, so tests can tell the icons apart. */
  readonly name: string;
  readonly className: string;
  readonly children: ReactNode;
}

function KindIcon({ name, className, children }: KindIconProps) {
  return (
    <svg
      viewBox={viewBox}
      fill="none"
      stroke="currentColor"
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
      data-icon={name}
      className={className}
    >
      {children}
    </svg>
  );
}

/** Props of {@link LibraryKindIcon}. */
export interface LibraryKindIconProps {
  /** The library kind; kinds of later API versions get the generic library icon. */
  readonly kind: LibraryKind;
  readonly className: string;
}

/** Icon of a library kind: a film strip for movies, a screen for shows. */
export function LibraryKindIcon({ kind, className }: LibraryKindIconProps) {
  switch (kind) {
    case "movies":
      return (
        <KindIcon name="movies" className={className}>
          <rect x="3" y="4" width="18" height="16" rx="2" />
          <path d="M7 4v16M17 4v16M3 9h4M3 15h4M17 9h4M17 15h4" />
        </KindIcon>
      );
    case "shows":
      return (
        <KindIcon name="shows" className={className}>
          <rect x="3" y="5" width="18" height="12" rx="2" />
          <path d="M8 21h8M12 17v4" />
        </KindIcon>
      );
    default:
      // Unreachable for the kinds this client knows; a newer server may send more.
      return <LibraryIcon className={className} />;
  }
}

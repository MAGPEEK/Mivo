import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import type { SiblingLink, SiblingLinks } from "../hooks/useSiblingLinks";
import type { AncestorView } from "../lib/itemView";
import { itemNamespace } from "../locales";

const navLinkClass =
  "rounded-sm text-text-muted underline-offset-4 hover:text-text hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent";

/** Props of {@link AncestorLinks}. */
export interface AncestorLinksProps {
  readonly ancestors: readonly AncestorView[];
}

/** Links from a season or episode back up to its show and season. */
export function AncestorLinks({ ancestors }: AncestorLinksProps) {
  const { t } = useTranslation(itemNamespace);
  if (ancestors.length === 0) {
    return null;
  }
  return (
    <nav aria-label={t("breadcrumb")}>
      <ol className="flex flex-wrap items-center gap-x-2 text-sm">
        {ancestors.map((ancestor, position) => (
          <li key={ancestor.id} className="flex items-center gap-2">
            {position > 0 && (
              <span aria-hidden="true" className="text-text-muted">
                ›
              </span>
            )}
            <Link to="/items/$itemId" params={{ itemId: ancestor.id }} className={navLinkClass}>
              {ancestor.title}
            </Link>
          </li>
        ))}
      </ol>
    </nav>
  );
}

/** Props of {@link SiblingArrows}. */
export interface SiblingArrowsProps {
  readonly links: SiblingLinks;
}

/**
 * Previous and next arrows. They are links, because they navigate; an end of the list shows no
 * arrow at all.
 */
export function SiblingArrows({ links }: SiblingArrowsProps) {
  const { t } = useTranslation(itemNamespace);
  if (links.previous === undefined && links.next === undefined) {
    return null;
  }
  return (
    <nav aria-label={t("siblings")} className="flex gap-2">
      {links.previous !== undefined && <Arrow link={links.previous} glyph="‹" />}
      {links.next !== undefined && <Arrow link={links.next} glyph="›" />}
    </nav>
  );
}

function Arrow({ link, glyph }: { readonly link: SiblingLink; readonly glyph: string }) {
  return (
    <Link
      to="/items/$itemId"
      params={{ itemId: link.itemId }}
      aria-label={link.label}
      className="grid size-9 place-items-center rounded-md border border-divider bg-surface text-xl text-text hover:bg-surface-raised focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
    >
      <span aria-hidden="true">{glyph}</span>
    </Link>
  );
}

import { useTranslation } from "react-i18next";

import type { AlphaBarEntry } from "../lib/letterOffsets";
import { libraryNamespace } from "../locales";

/** Props of {@link AlphaJumpBar}. */
export interface AlphaJumpBarProps {
  readonly entries: readonly AlphaBarEntry[];
  /** Called with the list position of the letter's first item. */
  readonly onJump: (offset: number) => void;
}

const symbolsLetter = "#";

const buttonClass =
  "h-[18px] w-[22px] rounded-sm text-[11px] leading-none font-semibold text-text hover:bg-surface-raised hover:text-accent focus-visible:outline-2 focus-visible:outline-accent disabled:cursor-default disabled:text-text-muted disabled:opacity-40 disabled:hover:bg-transparent disabled:hover:text-text-muted";

/**
 * The right-edge `# A-Z` bar (LB-29): one button per letter that jumps to the first title
 * starting with it. Letters without a title stay in place but are dimmed and disabled, so they
 * leave the tab order.
 */
export function AlphaJumpBar({ entries, onJump }: AlphaJumpBarProps) {
  const { t } = useTranslation(libraryNamespace);
  return (
    <nav
      aria-label={t("alpha.label")}
      className="sticky top-20 flex max-h-[calc(100dvh-6rem)] shrink-0 flex-col gap-px self-start overflow-y-auto"
    >
      {entries.map(({ letter, offset }) => (
        <button
          key={letter}
          type="button"
          disabled={offset === undefined}
          aria-label={letter === symbolsLetter ? t("alpha.symbols") : letter}
          onClick={() => {
            if (offset !== undefined) {
              onJump(offset);
            }
          }}
          className={buttonClass}
        >
          {letter}
        </button>
      ))}
    </nav>
  );
}

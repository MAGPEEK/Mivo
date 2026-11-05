import type { KeyboardEvent } from "react";
import { useTranslation } from "react-i18next";

import { CloseIcon } from "../../../shared/ui";
import type { BrowseControls } from "../hooks/useBrowseState";
import { useFilterChips } from "../hooks/useFilterChips";
import { libraryNamespace } from "../locales";

/** Props of {@link FilterChips}. */
export interface FilterChipsProps {
  readonly browse: BrowseControls;
}

const chipClass =
  "inline-flex items-center gap-1.5 rounded-full border border-divider bg-surface-raised py-0.5 pr-1.5 pl-3 text-xs text-text hover:bg-surface";
const clearClass = "rounded-md px-2 py-1 text-xs font-medium text-text-muted hover:text-text";

function isRemoveKey(event: KeyboardEvent): boolean {
  return event.key === "Delete" || event.key === "Backspace";
}

/**
 * The active filters under the Browse toolbar (LB-20): one chip per quick filter or field value;
 * a click, Delete or Backspace removes it, "Clear all" removes every filter. Nothing renders
 * without filters.
 */
export function FilterChips({ browse }: FilterChipsProps) {
  const { t } = useTranslation(libraryNamespace);
  const { chips, remove, clearAll, chipRefs } = useFilterChips(browse);
  if (chips.length === 0) {
    return null;
  }
  return (
    <div
      role="group"
      aria-label={t("filter.chipsLabel")}
      className="-mt-3 mb-6 flex flex-wrap items-center gap-2"
    >
      <ul className="flex flex-wrap gap-2">
        {chips.map((chip, index) => (
          <li key={chip.key}>
            <button
              ref={(element) => {
                chipRefs.current[index] = element;
              }}
              type="button"
              aria-label={t("filter.removeChip", { label: chip.label })}
              onClick={() => {
                remove(chip, index);
              }}
              onKeyDown={(event) => {
                if (isRemoveKey(event)) {
                  event.preventDefault();
                  remove(chip, index);
                }
              }}
              className={chipClass}
            >
              {chip.label}
              <CloseIcon className="size-3 text-text-muted" />
            </button>
          </li>
        ))}
      </ul>
      <button type="button" onClick={clearAll} className={clearClass}>
        {t("filter.clearAll")}
      </button>
    </div>
  );
}

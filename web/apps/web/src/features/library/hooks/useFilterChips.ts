import { useLayoutEffect, useRef, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { filterChips, type FilterChipModel } from "../lib/filterChips";
import { libraryNamespace } from "../locales";
import type { BrowseControls } from "./useBrowseState";
import { useFilterLabels } from "./useFilterLabels";

/** The active filter chips and their actions. */
export interface FilterChips {
  readonly chips: readonly FilterChipModel[];
  /** Removes the chip's filter; focus moves to the chip that takes its place. */
  readonly remove: (chip: FilterChipModel, index: number) => void;
  readonly clearAll: () => void;
  readonly chipRefs: RefObject<(HTMLButtonElement | null)[]>;
}

/**
 * The chips of the active filters under the Browse toolbar. Removing a chip changes the URL;
 * once the shorter list renders, focus moves to the chip now at the removed position (or the
 * last one), so keyboard users can remove several in a row.
 */
export function useFilterChips(browse: BrowseControls): FilterChips {
  const { t } = useTranslation(libraryNamespace);
  const labels = useFilterLabels();
  const chips = filterChips(browse, labels, (field, value) => t("filter.chip", { field, value }));
  const chipRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const pendingFocus = useRef<number | undefined>(undefined);
  const count = chips.length;
  useLayoutEffect(() => {
    const index = pendingFocus.current;
    pendingFocus.current = undefined;
    if (index !== undefined && count > 0) {
      chipRefs.current[Math.min(index, count - 1)]?.focus();
    }
  }, [count]);
  return {
    chips,
    remove: (chip, index) => {
      pendingFocus.current = index;
      if (chip.target.kind === "quick") {
        browse.selectQuick("all");
      } else {
        browse.toggleFieldValue(chip.target.field, chip.target.value);
      }
    },
    clearAll: browse.clearFilters,
    chipRefs,
  };
}

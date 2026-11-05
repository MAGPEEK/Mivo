import { useLayoutEffect, useRef, useState, type KeyboardEvent, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { useMenu, type MenuPlacement, type MenuRefs } from "../../../shared/ui";
import type { FilterLabels } from "../lib/filterLabels";
import {
  filterMenuSections,
  type FilterMenuItem,
  type FilterMenuSection,
  type FilterMenuTexts,
  type FilterMenuView,
} from "../lib/filterMenuModel";
import { libraryNamespace } from "../locales";
import type { BrowseControls } from "./useBrowseState";
import { useFacets } from "./useFacets";
import { useFilterLabels } from "./useFilterLabels";

/** State and handlers of the filter menu button (LB-20). */
export interface FilterMenuState {
  readonly open: boolean;
  readonly placement: MenuPlacement;
  readonly sections: readonly FilterMenuSection[];
  /** Accessible name of the popup: "Filters" or the field whose values it shows. */
  readonly menuLabel: string;
  readonly triggerLabel: string;
  readonly onTriggerClick: () => void;
  readonly onTriggerKeyDown: (event: KeyboardEvent) => void;
  readonly onMenuKeyDown: (event: KeyboardEvent) => void;
  readonly choose: (item: FilterMenuItem) => void;
}

// After a view change: focus the first value of the new field view, or the item with this key.
type PendingFocus =
  { readonly kind: "firstValue" } | { readonly kind: "key"; readonly key: string };

function useMenuTexts(labels: FilterLabels): FilterMenuTexts {
  const { t } = useTranslation(libraryNamespace);
  return {
    labels,
    quickHeading: t("filter.quickHeading"),
    fieldHeading: t("filter.fieldHeading"),
    back: t("filter.back"),
    loading: t("filter.loading"),
    loadFailed: t("filter.loadFailed"),
    notSupported: t("filter.notSupported"),
    noValues: t("filter.noValues"),
    count: (count) => t("filter.count", { count }),
    valueWithCount: (label, count) => t("filter.valueWithCount", { label, count }),
    fieldWithSelection: (label, count) => t("filter.fieldWithSelection", { label, count }),
  };
}

function focusIndex(items: readonly FilterMenuItem[], pending: PendingFocus): number {
  if (pending.kind === "key") {
    return items.findIndex((item) => item.key === pending.key);
  }
  const firstValue = items.findIndex((item) => item.action?.kind === "toggleValue");
  // Without values yet (loading), the way back is the only enabled item.
  return Math.max(firstValue, 0);
}

// Moves focus once the new view has rendered; the menu hook focuses only when it opens.
function usePendingFocus(
  items: readonly FilterMenuItem[],
  itemRefs: RefObject<(HTMLButtonElement | null)[]>,
): (pending: PendingFocus) => void {
  const pendingFocus = useRef<PendingFocus | undefined>(undefined);
  useLayoutEffect(() => {
    const pending = pendingFocus.current;
    if (pending !== undefined) {
      pendingFocus.current = undefined;
      itemRefs.current[focusIndex(items, pending)]?.focus();
    }
  });
  return (pending) => {
    pendingFocus.current = pending;
  };
}

interface FilterMenuContent {
  readonly sections: readonly FilterMenuSection[];
  readonly menuLabel: string;
  readonly triggerLabel: string;
}

// The sections and labels of the menu in `view`; counts load once `used`.
function useFilterMenuContent(
  libraryId: string,
  browse: BrowseControls,
  view: FilterMenuView,
  used: boolean,
): FilterMenuContent {
  const { t } = useTranslation(libraryNamespace);
  const labels = useFilterLabels();
  const texts = useMenuTexts(labels);
  const facets = useFacets(libraryId, browse.facetQuery, used);
  const input = { view, quick: browse.quick, fields: browse.fields, facets };
  return {
    sections: filterMenuSections(input, texts),
    menuLabel: view === "root" ? t("filter.menuLabel") : labels.field(view),
    triggerLabel:
      browse.quick === "all"
        ? t("filter.trigger")
        : t("filter.triggerQuick", { quick: labels.quick(browse.quick) }),
  };
}

/**
 * The filter menu of the Browse toolbar: a menu button whose popup shows the quick filters and
 * the fields, and drills into a field to toggle its values (ArrowRight/ArrowLeft or the items).
 * Counts load on first use and follow every filter change from then on. The component owns the
 * element refs, so render code never reads them.
 */
export function useFilterMenu(
  libraryId: string,
  browse: BrowseControls,
  refs: MenuRefs,
): FilterMenuState {
  const [view, setView] = useState<FilterMenuView>("root");
  const [used, setUsed] = useState(false);
  const content = useFilterMenuContent(libraryId, browse, view, used);
  const items = content.sections.flatMap((section) => section.items);
  const menu = useMenu(
    {
      labels: items.map((item) => item.label),
      disabled: items.map((item) => item.action === undefined),
    },
    refs,
  );
  const focusAfterRender = usePendingFocus(items, refs.items);
  const showView = (next: FilterMenuView, pending: PendingFocus): void => {
    focusAfterRender(pending);
    setView(next);
  };
  const choose = (item: FilterMenuItem): void => {
    chooseItem(item, { browse, view, showView, close: menu.close });
  };
  const beforeOpen = (): void => {
    setUsed(true);
    setView("root");
  };
  return {
    ...content,
    open: menu.open,
    placement: menu.placement,
    onTriggerClick: () => {
      beforeOpen();
      menu.toggle();
    },
    onTriggerKeyDown: (event) => {
      beforeOpen();
      menu.onTriggerKeyDown(event);
    },
    onMenuKeyDown: (event) => {
      const current =
        items[refs.items.current.findIndex((element) => element === document.activeElement)];
      if (!handleDrillKey(event, current, { view, choose })) {
        menu.onMenuKeyDown(event);
      }
    },
    choose,
  };
}

interface ChooseContext {
  readonly browse: BrowseControls;
  readonly view: FilterMenuView;
  readonly showView: (next: FilterMenuView, pending: PendingFocus) => void;
  readonly close: (returnFocus: boolean) => void;
}

function chooseItem(item: FilterMenuItem, context: ChooseContext): void {
  const { browse, view, showView, close } = context;
  const action = item.action;
  switch (action?.kind) {
    case "quick":
      close(true);
      browse.selectQuick(action.id);
      break;
    case "openField":
      showView(action.field, { kind: "firstValue" });
      break;
    case "back":
      showView("root", { kind: "key", key: `field-${view}` });
      break;
    case "toggleValue":
      // The menu stays open, so several values can be chosen in a row.
      browse.toggleFieldValue(action.field, action.value);
      break;
    case undefined:
      break;
  }
}

interface DrillContext {
  readonly view: FilterMenuView;
  readonly choose: (item: FilterMenuItem) => void;
}

// ArrowRight opens the focused field, ArrowLeft goes back to the root view; true when handled.
function handleDrillKey(
  event: KeyboardEvent,
  current: FilterMenuItem | undefined,
  { view, choose }: DrillContext,
): boolean {
  if (event.key === "ArrowRight" && current?.action?.kind === "openField") {
    event.preventDefault();
    choose(current);
    return true;
  }
  if (event.key === "ArrowLeft" && view !== "root") {
    event.preventDefault();
    choose({ key: "back", role: "menuitem", label: "", action: { kind: "back" } });
    return true;
  }
  return false;
}

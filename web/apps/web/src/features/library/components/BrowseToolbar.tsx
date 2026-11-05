import { belowBreakpointQuery } from "@mivo/ui-tokens";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { Menu, SegmentedControl, Sheet, useMediaQuery } from "../../../shared/ui";
import type { BrowseControls } from "../hooks/useBrowseState";
import { libraryItemFilters } from "../lib/browseQuery";
import { libraryNamespace } from "../locales";
import { secondaryButtonClass } from "./buttonClasses";

/** Props of {@link BrowseToolbar}. */
export interface BrowseToolbarProps {
  readonly browse: BrowseControls;
  /** Items matching the current query; absent hides the count. */
  readonly totalCount: number | undefined;
}

// Phones get the controls in a sheet, as in the approved prototype (`browse.html`).
const phoneScreenQuery = belowBreakpointQuery("sm");

/**
 * Toolbar of the Browse pivot (LB-20...23): quick filter, type switch, sort with direction and
 * the item count. One line on wider screens; on phones the controls sit in a sheet.
 */
export function BrowseToolbar({ browse, totalCount }: BrowseToolbarProps) {
  const { t } = useTranslation(libraryNamespace);
  const phone = useMediaQuery(phoneScreenQuery);
  return (
    <div
      role="group"
      aria-label={t("browse.label")}
      className="mb-6 flex flex-wrap items-center gap-3"
    >
      {phone ? <PhoneControls browse={browse} /> : <BrowseControlsLine browse={browse} />}
      <ItemCount type={browse.type} totalCount={totalCount} />
    </div>
  );
}

function PhoneControls({ browse }: { readonly browse: BrowseControls }) {
  const { t } = useTranslation(libraryNamespace);
  const [open, setOpen] = useState(false);
  return (
    <>
      <button
        type="button"
        aria-haspopup="dialog"
        onClick={() => {
          setOpen(true);
        }}
        className={secondaryButtonClass}
      >
        {t("browse.sheetTitle")}
      </button>
      {open && (
        <Sheet
          title={t("browse.sheetTitle")}
          closeLabel={t("browse.close")}
          onClose={() => {
            setOpen(false);
          }}
        >
          <div className="flex flex-col items-start gap-4 p-5">
            <BrowseControlsLine browse={browse} />
          </div>
        </Sheet>
      )}
    </>
  );
}

function BrowseControlsLine({ browse }: { readonly browse: BrowseControls }) {
  const { t } = useTranslation(libraryNamespace);
  return (
    <>
      <SegmentedControl
        label={t("filter.label")}
        options={libraryItemFilters.map((id) => ({ id, label: t(`filter.${id}`) }))}
        value={browse.filter}
        onChange={browse.selectFilter}
      />
      {browse.types.length > 1 && (
        <Menu
          trigger={t("browse.typeTrigger", { type: t(`browse.type.${browse.type}`) })}
          entries={browse.types.map((type) => ({
            id: type,
            label: t(`browse.type.${type}`),
            kind: "radio",
            checked: type === browse.type,
            onSelect: () => {
              browse.selectType(type);
            },
          }))}
        />
      )}
      <SortControls browse={browse} />
    </>
  );
}

function SortControls({ browse }: { readonly browse: BrowseControls }) {
  const { t } = useTranslation(libraryNamespace);
  const orderLabel = t(`browse.order.${browse.order}`);
  return (
    <div className="flex items-center gap-1">
      <Menu
        trigger={t("browse.sortTrigger", { sort: t(`browse.sort.${browse.sort}`) })}
        entries={browse.sorts.map((sort) => ({
          id: sort,
          label: t(`browse.sort.${sort}`),
          kind: "radio",
          checked: sort === browse.sort,
          onSelect: () => {
            browse.selectSort(sort);
          },
        }))}
      />
      {/* A shuffled list has no direction (the API ignores order for "random"). */}
      {browse.sort !== "random" && (
        <button
          type="button"
          onClick={browse.toggleOrder}
          aria-label={t("browse.orderToggle", { order: orderLabel })}
          className={secondaryButtonClass}
        >
          <span aria-hidden="true">{browse.order === "desc" ? "↓" : "↑"}</span>
          <span className="sr-only sm:not-sr-only sm:ml-1">{orderLabel}</span>
        </button>
      )}
    </div>
  );
}

interface ItemCountProps {
  readonly type: BrowseControls["type"];
  readonly totalCount: number | undefined;
}

function ItemCount({ type, totalCount }: ItemCountProps) {
  const { t } = useTranslation(libraryNamespace);
  return (
    // Polite live region, so a new filter or type announces the new count.
    <p aria-live="polite" className="text-sm text-text-muted sm:ml-auto">
      {totalCount === undefined ? "" : t(`browse.count.${type}`, { count: totalCount })}
    </p>
  );
}

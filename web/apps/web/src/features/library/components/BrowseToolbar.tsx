import { belowBreakpointQuery } from "@mivo/ui-tokens";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { Menu, Sheet, useMediaQuery } from "../../../shared/ui";
import type { BrowseControls } from "../hooks/useBrowseState";
import { libraryNamespace } from "../locales";
import { secondaryButtonClass } from "./buttonClasses";
import { FilterMenu } from "./FilterMenu";
import { ViewControls } from "./ViewControls";
import type { ViewPreference } from "../hooks/useViewPreference";

/** Props of {@link BrowseToolbar}. */
export interface BrowseToolbarProps {
  readonly libraryId: string;
  readonly browse: BrowseControls;
  /** Items matching the current query; absent hides the count. */
  readonly totalCount: number | undefined;
  /** The device's view style and card size; absent hides the view controls. */
  readonly view?: ViewPreference | undefined;
}

// Phones get the controls in a sheet, as in the approved prototype (`browse.html`).
const phoneScreenQuery = belowBreakpointQuery("sm");

interface ControlsProps {
  readonly libraryId: string;
  readonly browse: BrowseControls;
}

/**
 * Toolbar of the Browse pivot (LB-20...23): filter menu, type switch, sort with direction and
 * the item count. One line on wider screens; on phones the controls sit in a sheet.
 */
export function BrowseToolbar({ libraryId, browse, totalCount, view }: BrowseToolbarProps) {
  const { t } = useTranslation(libraryNamespace);
  const phone = useMediaQuery(phoneScreenQuery);
  return (
    <div
      role="group"
      aria-label={t("browse.label")}
      className="mb-6 flex flex-wrap items-center gap-3"
    >
      {phone ? (
        <PhoneControls libraryId={libraryId} browse={browse} />
      ) : (
        <BrowseControlsLine libraryId={libraryId} browse={browse} />
      )}
      {view !== undefined && <ViewControls view={view} />}
      <ItemCount type={browse.type} totalCount={totalCount} />
    </div>
  );
}

function PhoneControls({ libraryId, browse }: ControlsProps) {
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
            <BrowseControlsLine libraryId={libraryId} browse={browse} />
          </div>
        </Sheet>
      )}
    </>
  );
}

function BrowseControlsLine({ libraryId, browse }: ControlsProps) {
  const { t } = useTranslation(libraryNamespace);
  return (
    <>
      <FilterMenu libraryId={libraryId} browse={browse} />
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

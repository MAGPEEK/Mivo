import { useId, useRef, type RefObject } from "react";

import { secondaryButtonClass } from "../../../shared/ui";
import type { BrowseControls } from "../hooks/useBrowseState";
import { useFilterMenu, type FilterMenuState } from "../hooks/useFilterMenu";
import type { FilterMenuItem, FilterMenuSection } from "../lib/filterMenuModel";

/** Props of {@link FilterMenu}. */
export interface FilterMenuProps {
  readonly libraryId: string;
  readonly browse: BrowseControls;
}

type ItemRefs = RefObject<(HTMLButtonElement | null)[]>;

const itemClass =
  "flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm text-text hover:bg-divider disabled:cursor-not-allowed disabled:text-text-muted";
const headingClass = "px-3 pt-2 pb-1 text-xs font-semibold tracking-wide text-text-muted uppercase";

/**
 * Filter menu of the Browse toolbar (LB-20): quick filters as a single choice, field filters
 * whose values (with their counts) combine. A menu button after the WAI-ARIA pattern; a field
 * opens its values in the same popup.
 */
export function FilterMenu({ libraryId, browse }: FilterMenuProps) {
  const menuId = useId();
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const menu = useFilterMenu(libraryId, browse, {
    trigger: triggerRef,
    menu: menuRef,
    items: itemRefs,
  });
  return (
    <div className="relative inline-block">
      <button
        ref={triggerRef}
        type="button"
        aria-haspopup="menu"
        aria-expanded={menu.open}
        aria-controls={menu.open ? menuId : undefined}
        onClick={menu.onTriggerClick}
        onKeyDown={menu.onTriggerKeyDown}
        className={secondaryButtonClass}
      >
        <FilterIcon />
        {menu.triggerLabel}
      </button>
      {menu.open && <FilterPopup id={menuId} menu={menu} popupRef={menuRef} itemRefs={itemRefs} />}
    </div>
  );
}

function FilterIcon() {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinejoin="round"
      className="mr-1.5 inline size-4 align-[-3px]"
    >
      <path d="M3 5h18l-7 8v6l-4 2v-8z" />
    </svg>
  );
}

interface FilterPopupProps {
  readonly id: string;
  readonly menu: FilterMenuState;
  readonly popupRef: RefObject<HTMLDivElement | null>;
  readonly itemRefs: ItemRefs;
}

function FilterPopup({ id, menu, popupRef, itemRefs }: FilterPopupProps) {
  const side = menu.placement === "up" ? "bottom-full mb-1" : "top-full mt-1";
  // Position of each section's first item among all items, for the focus refs.
  const firstIndices = menu.sections.map((_, sectionIndex) =>
    menu.sections
      .slice(0, sectionIndex)
      .reduce((count, previous) => count + previous.items.length, 0),
  );
  return (
    // The keyboard handler sits on the menu role, which is a composite widget.
    <div
      ref={popupRef}
      id={id}
      role="menu"
      aria-label={menu.menuLabel}
      tabIndex={-1}
      onKeyDown={menu.onMenuKeyDown}
      data-placement={menu.placement}
      className={`absolute left-0 z-40 max-h-96 min-w-56 overflow-y-auto rounded-md border border-divider bg-surface-raised py-1 shadow-overlay ${side}`}
    >
      {menu.sections.map((section, sectionIndex) => (
        <MenuSection
          key={section.key}
          section={section}
          separated={sectionIndex > 0}
          firstIndex={firstIndices[sectionIndex] ?? 0}
          itemRefs={itemRefs}
          onChosen={menu.choose}
        />
      ))}
    </div>
  );
}

interface MenuSectionProps {
  readonly section: FilterMenuSection;
  readonly separated: boolean;
  /** Position of the section's first item among all items, for the focus refs. */
  readonly firstIndex: number;
  readonly itemRefs: ItemRefs;
  readonly onChosen: (item: FilterMenuItem) => void;
}

function MenuSection({ section, separated, firstIndex, itemRefs, onChosen }: MenuSectionProps) {
  const headingId = useId();
  return (
    <>
      {separated && <div role="separator" className="my-1 border-t border-divider" />}
      <div role="group" aria-labelledby={section.heading === undefined ? undefined : headingId}>
        {section.heading !== undefined && (
          <div id={headingId} className={headingClass}>
            {section.heading}
          </div>
        )}
        {section.items.map((item, index) => (
          <MenuItem
            key={item.key}
            item={item}
            itemRef={(element) => {
              itemRefs.current[firstIndex + index] = element;
            }}
            onChosen={() => {
              onChosen(item);
            }}
          />
        ))}
      </div>
    </>
  );
}

interface MenuItemProps {
  readonly item: FilterMenuItem;
  readonly itemRef: (element: HTMLButtonElement | null) => void;
  readonly onChosen: () => void;
}

function MenuItem({ item, itemRef, onChosen }: MenuItemProps) {
  const checkable = item.role !== "menuitem";
  const opensField = item.action?.kind === "openField";
  const back = item.action?.kind === "back";
  return (
    <button
      ref={itemRef}
      type="button"
      role={item.role}
      aria-checked={checkable ? item.checked === true : undefined}
      aria-label={item.accessibleLabel}
      disabled={item.action === undefined}
      tabIndex={-1}
      onClick={onChosen}
      className={itemClass}
    >
      <span aria-hidden="true" className="w-4 text-accent">
        {checkable && item.checked === true ? "✓" : ""}
        {back ? "‹" : ""}
      </span>
      <span className="flex-1">{item.label}</span>
      {item.detail !== undefined && (
        <span aria-hidden="true" className="text-xs text-text-muted tabular-nums">
          {item.detail}
        </span>
      )}
      {opensField && (
        <span aria-hidden="true" className="text-text-muted">
          ›
        </span>
      )}
    </button>
  );
}

import { useId, useRef, type KeyboardEvent, type ReactNode, type RefObject } from "react";

import { secondaryButtonClass } from "./buttonClasses";
import { useMenu, type MenuPlacement } from "./useMenu";

/** One entry of a {@link Menu}. */
export interface MenuEntry {
  readonly id: string;
  readonly label: string;
  /** `radio` and `checkbox` entries show `checked`; the default is a plain action. */
  readonly kind?: "item" | "radio" | "checkbox";
  readonly checked?: boolean;
  readonly disabled?: boolean;
  readonly onSelect: () => void;
}

/** Props of {@link Menu}. */
export interface MenuProps {
  /** Content of the button; give `ariaLabel` when it is only an icon. */
  readonly trigger: ReactNode;
  readonly ariaLabel?: string;
  readonly entries: readonly MenuEntry[];
  /** Which edge of the button the popup aligns to. */
  readonly align?: "start" | "end";
  /**
   * Replaces the look of the button, e.g. for the menu half of a split button; the button
   * fills the height its parent gives the menu.
   */
  readonly triggerClassName?: string;
}

const roles = { item: "menuitem", radio: "menuitemradio", checkbox: "menuitemcheckbox" } as const;
const itemClass =
  "flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm text-text hover:bg-divider disabled:cursor-not-allowed disabled:text-text-muted";

/**
 * Button with a popup menu (WAI-ARIA menu button). Arrow keys, Home/End and type-ahead move
 * focus, Enter/Space choose, Escape closes and returns focus to the button. The popup opens
 * upward when there is no room below.
 */
export function Menu(props: MenuProps) {
  const { trigger, ariaLabel, entries, align = "start" } = props;
  const menuId = useId();
  const triggerId = useId();
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const state = useMenu(
    {
      labels: entries.map((entry) => entry.label),
      disabled: entries.map((entry) => entry.disabled === true),
    },
    { trigger: triggerRef, menu: menuRef, items: itemRefs },
  );
  return (
    <div className="relative inline-flex">
      <button
        ref={triggerRef}
        id={triggerId}
        type="button"
        aria-haspopup="menu"
        aria-expanded={state.open}
        aria-controls={state.open ? menuId : undefined}
        aria-label={ariaLabel}
        onClick={state.toggle}
        onKeyDown={state.onTriggerKeyDown}
        className={props.triggerClassName ?? secondaryButtonClass}
      >
        {trigger}
      </button>
      {state.open && (
        <MenuPopup
          popupRef={menuRef}
          itemRefs={itemRefs}
          id={menuId}
          triggerId={triggerId}
          ariaLabel={ariaLabel}
          entries={entries}
          placement={state.placement}
          align={align}
          onKeyDown={state.onMenuKeyDown}
          onChosen={(entry) => {
            state.close(true);
            entry.onSelect();
          }}
        />
      )}
    </div>
  );
}

interface MenuPopupProps {
  readonly popupRef: RefObject<HTMLDivElement | null>;
  readonly itemRefs: RefObject<(HTMLButtonElement | null)[]>;
  readonly id: string;
  /** Names the menu after its button when there is no explicit `ariaLabel`. */
  readonly triggerId: string;
  readonly ariaLabel: string | undefined;
  readonly entries: readonly MenuEntry[];
  readonly placement: MenuPlacement;
  readonly align: "start" | "end";
  readonly onKeyDown: (event: KeyboardEvent) => void;
  readonly onChosen: (entry: MenuEntry) => void;
}

function MenuPopup(props: MenuPopupProps) {
  const { popupRef, itemRefs, id, triggerId, ariaLabel, entries, placement, align } = props;
  const side = placement === "up" ? "bottom-full mb-1" : "top-full mt-1";
  const edge = align === "end" ? "right-0" : "left-0";
  return (
    // The keyboard handler sits on the menu role, which is a composite widget.
    <div
      ref={popupRef}
      id={id}
      role="menu"
      aria-label={ariaLabel}
      aria-labelledby={ariaLabel === undefined ? triggerId : undefined}
      tabIndex={-1}
      onKeyDown={props.onKeyDown}
      data-placement={placement}
      className={`absolute z-40 min-w-44 rounded-md border border-divider bg-surface-raised py-1 shadow-overlay ${side} ${edge}`}
    >
      {entries.map((entry, index) => (
        <MenuItem
          key={entry.id}
          entry={entry}
          itemRef={(element) => {
            itemRefs.current[index] = element;
          }}
          onChosen={() => {
            props.onChosen(entry);
          }}
        />
      ))}
    </div>
  );
}

interface MenuItemProps {
  readonly entry: MenuEntry;
  readonly itemRef: (element: HTMLButtonElement | null) => void;
  readonly onChosen: () => void;
}

function MenuItem({ entry, itemRef, onChosen }: MenuItemProps) {
  const kind = entry.kind ?? "item";
  const checked = kind !== "item" && entry.checked === true;
  return (
    <button
      ref={itemRef}
      type="button"
      role={roles[kind]}
      aria-checked={kind === "item" ? undefined : checked}
      disabled={entry.disabled}
      tabIndex={-1}
      onClick={onChosen}
      className={itemClass}
    >
      <span aria-hidden="true" className="w-4 text-accent">
        {checked ? "✓" : ""}
      </span>
      {entry.label}
    </button>
  );
}

import type { KeyboardEvent, ReactNode } from "react";

import { rovingTarget } from "../../../shared/ui/rovingFocus";

/** Props of {@link OptionMenu}. */
export interface OptionMenuProps {
  /** Id of the heading that names the menu. */
  readonly labelledBy: string;
  readonly children: ReactNode;
}

const itemSelector = '[role="menuitemradio"]';

/** Arrow keys move focus between the items without choosing: a choice restarts the stream. */
function moveFocus(event: KeyboardEvent<HTMLElement>): void {
  const items = [...event.currentTarget.querySelectorAll<HTMLElement>(itemSelector)];
  const current = items.findIndex((item) => item === document.activeElement);
  const target =
    current < 0 ? undefined : rovingTarget(event.key, current, items.length, "vertical");
  if (target !== undefined) {
    event.preventDefault();
    items[target]?.focus();
  }
}

/**
 * A list of exclusive choices with menu semantics. Unlike a radio group, the arrow keys only
 * move focus; Enter or Space chooses (the prototype's decision, because choosing an audio
 * track or a burned-in subtitle restarts the stream and arrowing through the list must not).
 */
export function OptionMenu({ labelledBy, children }: OptionMenuProps) {
  return (
    <div
      role="menu"
      tabIndex={-1}
      aria-labelledby={labelledBy}
      onKeyDown={moveFocus}
      className="flex flex-col gap-0.5"
    >
      {children}
    </div>
  );
}

/** Props of {@link OptionItem}. */
export interface OptionItemProps {
  readonly checked: boolean;
  /** Whether Tab reaches this item: the checked one, or the first when none is checked. */
  readonly tabbable: boolean;
  /** Focus lands here when the panel opens. */
  readonly initialFocus?: boolean;
  readonly onChoose: () => void;
  readonly children: ReactNode;
}

/** One choice of an {@link OptionMenu}; the content is its accessible name. */
export function OptionItem({
  checked,
  tabbable,
  initialFocus = false,
  onChoose,
  children,
}: OptionItemProps) {
  return (
    <button
      type="button"
      role="menuitemradio"
      aria-checked={checked}
      tabIndex={tabbable ? 0 : -1}
      data-autofocus={initialFocus ? "" : undefined}
      onClick={onChoose}
      className={`flex w-full items-start gap-2 rounded-md px-2 py-1.5 text-start text-sm hover:bg-divider ${checked ? "bg-surface" : ""}`}
    >
      <span
        aria-hidden="true"
        className={`mt-1 size-3 shrink-0 rounded-full border-2 ${checked ? "border-accent bg-accent" : "border-text-muted"}`}
      />
      <span className="flex min-w-0 flex-col">{children}</span>
    </button>
  );
}

/** The index in `checked` flags Tab should reach: the checked item, else the first. */
export function tabbableIndex(checked: readonly boolean[]): number {
  return Math.max(0, checked.indexOf(true));
}

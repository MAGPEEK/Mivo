import { useId, type KeyboardEvent, type ReactNode } from "react";

/** One tab of {@link DialogTabs}. */
export interface DialogTab<Id extends string> {
  readonly id: Id;
  readonly label: string;
}

/** Props of {@link DialogTabs}. */
export interface DialogTabsProps<Id extends string> {
  readonly label: string;
  readonly tabs: readonly DialogTab<Id>[];
  readonly selected: Id;
  readonly onSelect: (id: Id) => void;
  /** The content of the selected tab. */
  readonly children: ReactNode;
}

const arrowStep: Readonly<Record<string, number>> = { ArrowRight: 1, ArrowLeft: -1 };

/**
 * Tab list with one panel. Arrow keys move between tabs (every tab stays in the Tab order, which
 * keeps the focus cycle of the dialog simple); only the selected panel is rendered.
 */
export function DialogTabs<Id extends string>({
  label,
  tabs,
  selected,
  onSelect,
  children,
}: DialogTabsProps<Id>) {
  const baseId = useId();
  const tabId = (id: Id) => `${baseId}-tab-${id}`;
  const onKeyDown = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    const step = arrowStep[event.key];
    const target =
      step === undefined ? undefined : tabs[(index + step + tabs.length) % tabs.length];
    if (target !== undefined) {
      event.preventDefault();
      onSelect(target.id);
      document.getElementById(tabId(target.id))?.focus();
    }
  };
  return (
    <>
      <div role="tablist" aria-label={label} className="mb-4 flex gap-1 border-b border-divider">
        {tabs.map((tab, index) => (
          <button
            key={tab.id}
            id={tabId(tab.id)}
            type="button"
            role="tab"
            aria-selected={tab.id === selected}
            aria-controls={`${baseId}-panel`}
            onClick={() => {
              onSelect(tab.id);
            }}
            onKeyDown={(event) => {
              onKeyDown(event, index);
            }}
            className="-mb-px border-b-2 border-transparent px-3 py-2 text-sm text-text-muted aria-selected:border-accent aria-selected:text-text"
          >
            {tab.label}
          </button>
        ))}
      </div>
      <div id={`${baseId}-panel`} role="tabpanel" aria-labelledby={tabId(selected)}>
        {children}
      </div>
    </>
  );
}

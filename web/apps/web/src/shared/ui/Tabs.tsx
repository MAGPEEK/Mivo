import { useId, type ReactNode } from "react";

import { useRovingFocus } from "./rovingFocus";

/** One tab of {@link Tabs}. */
export interface TabItem {
  readonly id: string;
  readonly label: string;
}

/** Props of {@link Tabs}. */
export interface TabsProps {
  /** Accessible name of the tab list. */
  readonly label: string;
  readonly tabs: readonly TabItem[];
  /** Id of the selected tab. */
  readonly value: string;
  readonly onChange: (id: string) => void;
  /** Horizontal tabs react to Left/Right, vertical ones to Up/Down; both to Home/End. */
  readonly orientation?: "horizontal" | "vertical";
  /** Content of the selected tab; rendered in a `tabpanel` labelled by that tab. */
  readonly children?: ReactNode;
}

const listClasses = {
  horizontal: "flex gap-1 border-b border-divider",
  vertical: "flex flex-col gap-1 border-l border-divider",
} as const;
const tabClasses = {
  horizontal: "-mb-px border-b-2 px-3 py-2 text-sm font-medium",
  vertical: "-ml-px border-l-2 px-3 py-2 text-left text-sm font-medium",
} as const;
const selectedClass = "border-accent text-text";
const idleClass = "border-transparent text-text-muted hover:text-text";

/** Tab pivot with roving tabindex; arrow keys move focus and select (automatic activation). */
export function Tabs(props: TabsProps) {
  const { label, tabs, value, onChange, orientation = "horizontal", children } = props;
  const base = useId();
  const selectedIndex = Math.max(
    0,
    tabs.findIndex((tab) => tab.id === value),
  );
  const { itemProps } = useRovingFocus({
    count: tabs.length,
    tabbableIndex: selectedIndex,
    orientation,
    onMove: (index) => {
      onChange(tabs[index]?.id ?? value);
    },
  });
  const panelId = children === undefined ? undefined : `${base}-panel`;
  return (
    <div className={orientation === "vertical" ? "flex gap-4" : undefined}>
      <div
        role="tablist"
        aria-label={label}
        aria-orientation={orientation}
        className={listClasses[orientation]}
      >
        {tabs.map((tab, index) => (
          <button
            key={tab.id}
            {...itemProps(index)}
            type="button"
            role="tab"
            id={`${base}-tab-${tab.id}`}
            aria-selected={index === selectedIndex}
            aria-controls={index === selectedIndex ? panelId : undefined}
            onClick={() => {
              onChange(tab.id);
            }}
            className={`${tabClasses[orientation]} ${index === selectedIndex ? selectedClass : idleClass}`}
          >
            {tab.label}
          </button>
        ))}
      </div>
      {panelId !== undefined && (
        <TabPanel id={panelId} tabId={`${base}-tab-${tabs[selectedIndex]?.id ?? ""}`}>
          {children}
        </TabPanel>
      )}
    </div>
  );
}

function TabPanel({ id, tabId, children }: { id: string; tabId: string; children: ReactNode }) {
  return (
    <div role="tabpanel" id={id} aria-labelledby={tabId}>
      {children}
    </div>
  );
}

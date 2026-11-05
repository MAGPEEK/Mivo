import { useRovingFocus } from "./rovingFocus";

/** One option of {@link SegmentedControl}. */
export interface SegmentOption {
  readonly id: string;
  readonly label: string;
}

/** Props of {@link SegmentedControl}. */
export interface SegmentedControlProps {
  /** Accessible name of the group, e.g. "View style". */
  readonly label: string;
  readonly options: readonly SegmentOption[];
  readonly value: string;
  readonly onChange: (id: string) => void;
}

/** Exclusive choice shown as joined buttons; a radio group with roving tabindex. */
export function SegmentedControl({ label, options, value, onChange }: SegmentedControlProps) {
  // Without a matching value nothing is checked, but the first option stays reachable by Tab.
  const tabbableIndex = Math.max(
    0,
    options.findIndex((option) => option.id === value),
  );
  const { itemProps } = useRovingFocus({
    count: options.length,
    tabbableIndex,
    onMove: (index) => {
      const option = options[index];
      if (option !== undefined) {
        onChange(option.id);
      }
    },
  });
  return (
    <div
      role="radiogroup"
      aria-label={label}
      className="inline-flex overflow-hidden rounded-md border border-divider bg-surface"
    >
      {options.map((option, index) => {
        const checked = option.id === value;
        return (
          <button
            key={option.id}
            {...itemProps(index)}
            type="button"
            role="radio"
            aria-checked={checked}
            onClick={() => {
              onChange(option.id);
            }}
            className={`px-3 py-1.5 text-sm font-medium ${checked ? "bg-accent text-on-accent" : "text-text hover:bg-surface-raised"}`}
          >
            {option.label}
          </button>
        );
      })}
    </div>
  );
}

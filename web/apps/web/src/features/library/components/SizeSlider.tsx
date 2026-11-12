import { useTranslation } from "react-i18next";

import { cardColumnWidths, cardStepCount } from "../lib/viewStyle";
import { libraryNamespace } from "../locales";

/** Props of {@link SizeSlider}. */
export interface SizeSliderProps {
  /** Current step, 0 (smallest) to 4. */
  readonly step: number;
  readonly onChange: (step: number) => void;
}

/**
 * The card size slider (LB-28): five discrete steps, operable with the arrow keys. The steps
 * map to fixed card widths, so the image requests stay within the allowed sizes.
 */
export function SizeSlider({ step, onChange }: SizeSliderProps) {
  const { t } = useTranslation(libraryNamespace);
  return (
    <label className="flex items-center gap-2 text-sm text-text-muted">
      <span>{t("view.size")}</span>
      <input
        type="range"
        min={0}
        max={cardStepCount - 1}
        step={1}
        value={step}
        aria-valuetext={t("view.sizeValue", {
          step: step + 1,
          total: cardStepCount,
          width: cardColumnWidths[step],
        })}
        onChange={(event) => {
          onChange(event.currentTarget.valueAsNumber);
        }}
        className="w-24 accent-accent"
      />
    </label>
  );
}

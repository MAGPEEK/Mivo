import { useTranslation } from "react-i18next";

import { SegmentedControl } from "../../../shared/ui";
import type { ViewPreference } from "../hooks/useViewPreference";
import { viewStyles } from "../lib/viewStyle";
import { libraryNamespace } from "../locales";
import { SizeSlider } from "./SizeSlider";

/** Props of {@link ViewControls}. */
export interface ViewControlsProps {
  readonly view: ViewPreference;
}

/** The view switch (Grid, List, Details) and, for the grid, the card size slider. */
export function ViewControls({ view }: ViewControlsProps) {
  const { t } = useTranslation(libraryNamespace);
  return (
    <>
      <SegmentedControl
        label={t("view.label")}
        options={viewStyles.map((style) => ({ id: style, label: t(`view.style.${style}`) }))}
        value={view.viewStyle}
        onChange={view.selectViewStyle}
      />
      {view.viewStyle === "grid" && (
        <SizeSlider step={view.cardStep} onChange={view.selectCardStep} />
      )}
    </>
  );
}

import { useCallback, useState } from "react";

import { browserLocalStorage } from "../../../shared/lib/persistentFlag";
import {
  cardStepStorageKey,
  clampCardStep,
  readCardStep,
  readViewStyle,
  viewStyles,
  viewStyleStorageKey,
  writePreference,
  type ViewStyle,
} from "../lib/viewStyle";

/** The device's view preference and the actions that change it. */
export interface ViewPreference {
  readonly viewStyle: ViewStyle;
  /** Slider step 0 (smallest cards) to 4. */
  readonly cardStep: number;
  /** Switches the view by id; unknown ids are ignored. */
  readonly selectViewStyle: (id: string) => void;
  readonly selectCardStep: (step: number) => void;
}

/**
 * The view style and card size of this device (LB-27, LB-28): read once from `localStorage`,
 * written on every change, shared by all libraries.
 */
export function useViewPreference(
  storage: Storage | undefined = browserLocalStorage(),
): ViewPreference {
  const [viewStyle, setViewStyle] = useState(() => readViewStyle(storage));
  const [cardStep, setCardStep] = useState(() => readCardStep(storage));
  const selectViewStyle = useCallback(
    (id: string) => {
      const next = viewStyles.find((style) => style === id);
      if (next !== undefined) {
        setViewStyle(next);
        writePreference(storage, viewStyleStorageKey, next);
      }
    },
    [storage],
  );
  const selectCardStep = useCallback(
    (step: number) => {
      const next = clampCardStep(step);
      setCardStep(next);
      writePreference(storage, cardStepStorageKey, String(next));
    },
    [storage],
  );
  return { viewStyle, cardStep, selectViewStyle, selectCardStep };
}

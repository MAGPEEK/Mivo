import { useId } from "react";
import { useTranslation } from "react-i18next";

import {
  contentLimitPresets,
  customAges,
  type ContentLimitDraft,
  type KnownAge,
  type KnownPreset,
} from "../lib/contentLimit";
import { usersNamespace } from "../locales";
import { inputClass } from "./TextInput";

/** Props of {@link ContentLimitField}. */
export interface ContentLimitFieldProps {
  readonly limit: ContentLimitDraft;
  readonly error?: string | undefined;
  readonly onPresetChange: (preset: KnownPreset) => void;
  readonly onCustomAgeChange: (age: KnownAge) => void;
  readonly onHideUnratedChange: (hide: boolean) => void;
}

function CustomAgeSelect({ limit, onCustomAgeChange }: ContentLimitFieldProps) {
  const { t } = useTranslation(usersNamespace);
  const id = useId();
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="text-sm text-text">
        {t("users.dialog.limit.customAge")}
      </label>
      <select
        id={id}
        value={limit.customAge}
        onChange={(event) => {
          const age = customAges.find((known) => String(known) === event.target.value);
          if (age !== undefined) {
            onCustomAgeChange(age);
          }
        }}
        className={inputClass}
      >
        {customAges.map((age) => (
          <option key={age} value={age}>
            {t("users.dialog.limit.age", { age })}
          </option>
        ))}
      </select>
    </div>
  );
}

/** Content limit presets, "Custom" with an age choice, and "Hide unrated titles" (US-11). */
export function ContentLimitField(props: ContentLimitFieldProps) {
  const { t } = useTranslation(usersNamespace);
  const presetId = useId();
  const errorId = useId();
  const { limit } = props;
  return (
    <fieldset className="mb-4 flex flex-col gap-2" aria-describedby={errorId}>
      <legend className="mb-1 text-sm font-medium text-text">
        {t("users.dialog.limit.legend")}
      </legend>
      <label htmlFor={presetId} className="text-sm text-text">
        {t("users.dialog.limit.preset")}
      </label>
      <select
        id={presetId}
        value={limit.preset}
        onChange={(event) => {
          const preset = contentLimitPresets.find((known) => known === event.target.value);
          if (preset !== undefined) {
            props.onPresetChange(preset);
          }
        }}
        className={inputClass}
      >
        {contentLimitPresets.map((preset) => (
          <option key={preset} value={preset}>
            {t(`users.limit.${preset}`)}
          </option>
        ))}
      </select>
      {limit.preset === "custom" && <CustomAgeSelect {...props} />}
      <label className="flex items-center gap-2 text-sm text-text">
        <input
          type="checkbox"
          checked={limit.hideUnrated}
          onChange={(event) => {
            props.onHideUnratedChange(event.target.checked);
          }}
        />
        {t("users.dialog.limit.hideUnrated")}
      </label>
      <p className="text-xs text-text-muted">{t("users.dialog.limit.enforce")}</p>
      <p id={errorId} role="alert" className="text-sm text-accent empty:hidden">
        {props.error}
      </p>
    </fieldset>
  );
}

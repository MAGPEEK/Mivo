import { useId } from "react";
import { useTranslation } from "react-i18next";

import { languageName } from "../../../item";
import { metadataLanguageCodes } from "../../lib/libraryDraft";
import { settingsNamespace } from "../../locales";
import type { MetadataSettings } from "../api/metadataSettings";
import type { MetadataSettingsControls } from "../hooks/useMetadataSettings";
import { metadataProblemKeys } from "../lib/problemKeys";
import {
  languageChoices,
  providerGroups,
  tmdbKeyStatus,
  toggleGroup,
  type ProviderGroup,
} from "../lib/providerGroups";
import { TmdbKeyField } from "./TmdbKeyField";

/** Props of the parts of {@link MetadataSettingsForm}. */
interface FormPartProps {
  readonly settings: MetadataSettings;
  readonly controls: MetadataSettingsControls;
}

const groupLabels = {
  tmdb: "metadata.providers.tmdb",
  music: "metadata.providers.music",
} as const;

const selectClass =
  "w-full max-w-sm rounded-md border border-divider bg-surface px-3 py-2 text-sm text-text";

// The controls stay enabled while a change saves: disabling them would drop keyboard focus.
// Each answer replaces the cached settings, so the last change wins.
function OnlineSwitch({ settings, controls }: FormPartProps) {
  const { t } = useTranslation(settingsNamespace);
  const hintId = useId();
  return (
    <fieldset className="flex flex-col gap-1">
      <legend className="mb-2 text-base font-semibold">{t("metadata.online.legend")}</legend>
      <label className="flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          role="switch"
          checked={settings.onlineEnabled}
          aria-describedby={hintId}
          onChange={(event) => {
            controls.update({ onlineEnabled: event.target.checked });
          }}
        />
        {t("metadata.online.label")}
      </label>
      <p id={hintId} className="pl-6 text-xs text-text-muted">
        {t("metadata.online.hint")}
      </p>
    </fieldset>
  );
}

interface GroupSwitchProps extends FormPartProps {
  readonly group: ProviderGroup;
}

function GroupSwitch({ settings, controls, group }: GroupSwitchProps) {
  const { t } = useTranslation(settingsNamespace);
  return (
    <div className="flex flex-col gap-2">
      <label className="flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          role="switch"
          checked={group.enabled}
          disabled={!settings.onlineEnabled}
          onChange={(event) => {
            controls.update(toggleGroup(group, event.target.checked));
          }}
        />
        {t(groupLabels[group.id])}
      </label>
      {group.id === "tmdb" && <TmdbKeyField keyStatus={tmdbKeyStatus(settings)} />}
    </div>
  );
}

function ProviderSwitches({ settings, controls }: FormPartProps) {
  const { t } = useTranslation(settingsNamespace);
  return (
    <fieldset className="flex flex-col gap-3">
      <legend className="mb-2 text-base font-semibold">{t("metadata.providers.legend")}</legend>
      {!settings.onlineEnabled && (
        <p className="text-xs text-text-muted">{t("metadata.providers.offHint")}</p>
      )}
      {providerGroups(settings).map((group) => (
        <GroupSwitch key={group.id} settings={settings} controls={controls} group={group} />
      ))}
    </fieldset>
  );
}

function LanguageField({ settings, controls }: FormPartProps) {
  const { t, i18n } = useTranslation(settingsNamespace);
  const selectId = useId();
  const hintId = useId();
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={selectId} className="text-base font-semibold">
        {t("metadata.language.label")}
      </label>
      <select
        id={selectId}
        value={settings.language}
        aria-describedby={hintId}
        onChange={(event) => {
          controls.update({ language: event.target.value });
        }}
        className={selectClass}
      >
        {languageChoices(metadataLanguageCodes, settings.language).map((code) => (
          <option key={code} value={code}>
            {code === "" ? t("metadata.language.default") : languageName(code, i18n.language)}
          </option>
        ))}
      </select>
      <p id={hintId} className="text-xs text-text-muted">
        {t("metadata.language.hint")}
      </p>
    </div>
  );
}

/** Props of {@link MetadataSettingsForm}. */
export type MetadataSettingsFormProps = FormPartProps;

/** The switches, key field and language of online metadata. Every change saves at once. */
export function MetadataSettingsForm({ settings, controls }: MetadataSettingsFormProps) {
  const { t } = useTranslation(settingsNamespace);
  return (
    <div className="flex flex-col gap-6 rounded-lg border border-divider bg-surface p-4">
      <OnlineSwitch settings={settings} controls={controls} />
      <ProviderSwitches settings={settings} controls={controls} />
      <LanguageField settings={settings} controls={controls} />
      <p role="status" className="text-sm text-text-muted">
        {controls.pending ? t("metadata.saving") : ""}
      </p>
      {controls.problem !== null && (
        <p role="alert" className="text-sm text-accent">
          {t(metadataProblemKeys[controls.problem])}
        </p>
      )}
    </div>
  );
}

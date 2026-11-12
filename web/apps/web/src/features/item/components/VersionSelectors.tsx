import { useId } from "react";
import { useTranslation } from "react-i18next";

import type { PlayChoiceView } from "../hooks/usePlayChoice";
import type { ChoiceOption } from "../lib/versions";
import { itemNamespace } from "../locales";

/** Props of {@link VersionSelectors}. */
export interface VersionSelectorsProps {
  readonly choice: PlayChoiceView;
}

const selectClass =
  "w-full min-w-44 rounded-md border border-divider bg-surface px-3 py-2 text-sm text-text";

/**
 * The version, audio and subtitle pickers before Play (DT-07). Each appears only when there
 * is something to choose: several versions, several audio tracks, any subtitle track. The
 * subtitle picker always offers "Off".
 */
export function VersionSelectors({ choice }: VersionSelectorsProps) {
  const { t } = useTranslation(itemNamespace);
  const { resolved, pickVersion, pickAudio, pickSubtitle } = choice;
  if (resolved === undefined) {
    return null;
  }
  const { version } = resolved;
  const showVersions = choice.versionOptions.length > 1;
  const showAudio = version.audio.length > 1;
  const showSubtitles = version.subtitles.length > 0;
  if (!showVersions && !showAudio && !showSubtitles) {
    return null;
  }
  return (
    <div
      role="group"
      aria-label={t("selectors.label")}
      className="grid max-w-3xl gap-3 sm:grid-cols-[repeat(auto-fit,minmax(11rem,1fr))]"
    >
      {showVersions && (
        <Selector
          label={t("selectors.version")}
          value={version.fileId}
          options={choice.versionOptions}
          onChange={pickVersion}
        />
      )}
      {showAudio && (
        <Selector
          label={t("selectors.audio")}
          value={resolved.audio ?? ""}
          options={version.audio}
          onChange={pickAudio}
        />
      )}
      {showSubtitles && (
        <Selector
          label={t("selectors.subtitles")}
          value={resolved.subtitle}
          options={[{ value: "", label: t("selectors.subtitlesOff") }, ...version.subtitles]}
          onChange={pickSubtitle}
        />
      )}
    </div>
  );
}

interface SelectorProps {
  readonly label: string;
  readonly value: string;
  readonly options: readonly ChoiceOption[];
  readonly onChange: (value: string) => void;
}

function Selector({ label, value, options, onChange }: SelectorProps) {
  const id = useId();
  return (
    <div className="flex min-w-0 flex-col gap-1">
      <label htmlFor={id} className="text-sm text-text-muted">
        {label}
      </label>
      <select
        id={id}
        value={value}
        onChange={(event) => {
          onChange(event.target.value);
        }}
        className={selectClass}
      >
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </div>
  );
}

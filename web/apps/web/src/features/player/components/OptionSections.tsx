import { resolutionLabel } from "@mivo/player-core";
import { useId } from "react";
import { useTranslation } from "react-i18next";

import { SegmentedControl } from "../../../shared/ui";
import type { PlaybackOptionsActions, PlaybackOptionsView } from "../hooks/usePlaybackOptions";

import { languageName } from "../lib/languageName";
import type { AudioRow, QualityRow, SubtitleRow } from "../lib/optionRows";
import { isSubtitleSize, subtitleSizes } from "../lib/subtitleSizePreference";
import { playerNamespace } from "../locales";
import { OptionItem, OptionMenu, tabbableIndex } from "./OptionMenu";
import { SpeedControl } from "./SpeedControl";

/** Props every section of the panel takes. */
export interface OptionSectionProps {
  readonly view: PlaybackOptionsView;
  readonly actions: PlaybackOptionsActions;
}

const hintClass = "mt-2 text-xs text-text-muted";
const secondaryLineClass = "text-xs text-text-muted";
const badgeClass = "rounded-sm bg-surface-raised px-1 text-xs font-medium text-text-muted";

function SectionHeading({ id, children }: { readonly id: string; readonly children: string }) {
  return (
    <h3 id={id} className="mb-2 text-sm font-semibold text-text-muted">
      {children}
    </h3>
  );
}

/** The name of a quality row and, for the chosen Automatic, the step it plays now. */
function QualityRowText({
  row,
  view,
}: {
  readonly row: QualityRow;
  readonly view: PlaybackOptionsView;
}) {
  const { t } = useTranslation(playerNamespace);
  if (row.kind === "original") {
    return <span>{t("options.quality.original")}</span>;
  }
  if (row.kind === "limited") {
    return (
      <span>
        {t("options.quality.step", {
          resolution: resolutionLabel(row.height ?? 0),
          megabits: row.megabits ?? 0,
        })}
      </span>
    );
  }
  const now =
    view.automaticStep === undefined
      ? t("options.quality.original")
      : resolutionLabel(view.automaticStep.height ?? 0);
  return (
    <>
      <span>{t("options.quality.automatic")}</span>
      {row.current ? (
        <span className={secondaryLineClass}>
          {t("options.quality.automaticNow", { quality: now })}
        </span>
      ) : null}
    </>
  );
}

/** PL-10: Original, Automatic and the steps of the ladder below the source. */
export function QualitySection({ view, actions }: OptionSectionProps) {
  const { t } = useTranslation(playerNamespace);
  const headingId = useId();
  const tabbable = tabbableIndex(view.quality.map((row) => row.current));
  return (
    <section aria-labelledby={headingId}>
      <SectionHeading id={headingId}>{t("options.quality.heading")}</SectionHeading>
      <OptionMenu labelledBy={headingId}>
        {view.quality.map((row, position) => (
          <OptionItem
            key={`${row.kind}-${String(row.maxBitRate)}`}
            checked={row.current}
            tabbable={position === tabbable}
            onChoose={() => {
              actions.chooseQuality(row);
            }}
          >
            <QualityRowText row={row} view={view} />
          </OptionItem>
        ))}
      </OptionMenu>
      {view.hasSteps ? (
        <button
          type="button"
          aria-expanded={view.showAllSteps}
          onClick={actions.toggleAllSteps}
          className="mt-1 rounded-sm px-2 py-1 text-sm text-accent hover:text-accent-hover"
        >
          {view.showAllSteps ? t("options.quality.showLess") : t("options.quality.showAll")}
        </button>
      ) : null}
      <p className={hintClass}>
        {view.hasSteps ? t("options.quality.hint") : t("options.quality.hintNone")}
      </p>
    </section>
  );
}

/** PL-11: the audio tracks, language first. */
export function AudioSection({ view, actions }: OptionSectionProps) {
  const { t, i18n } = useTranslation(playerNamespace);
  const headingId = useId();
  const tabbable = tabbableIndex(view.audio.map((row) => row.current));
  const initialFocusIndex = view.audio.findIndex((row) => row.current);
  const rowLabel = (row: AudioRow): string =>
    languageName(row.language, i18n.language) ?? t("options.audio.unknownLanguage");
  return (
    <section aria-labelledby={headingId}>
      <SectionHeading id={headingId}>{t("options.audio.heading")}</SectionHeading>
      <OptionMenu labelledBy={headingId}>
        {view.audio.map((row, position) => (
          <OptionItem
            key={row.index}
            checked={row.current}
            tabbable={position === tabbable}
            initialFocus={position === initialFocusIndex}
            onChoose={() => {
              actions.chooseAudio(row.index);
            }}
          >
            <span>{rowLabel(row)}</span>
            <span className={secondaryLineClass}>
              {[row.title, row.details]
                .filter((part) => part !== undefined && part !== "")
                .join(" · ")}
            </span>
            {row.converted ? (
              <span className={secondaryLineClass}>{t("options.audio.converted")}</span>
            ) : null}
          </OptionItem>
        ))}
      </OptionMenu>
      <p className={hintClass}>{t(audioHintKey(view.audio.length))}</p>
    </section>
  );
}

function audioHintKey(
  count: number,
): "options.audio.none" | "options.audio.single" | "options.audio.restart" {
  if (count === 0) {
    return "options.audio.none";
  }
  return count === 1 ? "options.audio.single" : "options.audio.restart";
}

function SubtitleBadges({ row }: { readonly row: SubtitleRow }) {
  const { t } = useTranslation(playerNamespace);
  return (
    <span className="flex flex-wrap items-center gap-1">
      {row.forced ? <span className={badgeClass}>{t("options.subtitles.forced")}</span> : null}
      {row.sdh ? <span className={badgeClass}>{t("options.subtitles.sdh")}</span> : null}
      {row.external ? <span className={badgeClass}>{t("options.subtitles.external")}</span> : null}
      <span className={secondaryLineClass}>{row.format}</span>
    </span>
  );
}

/** The translation key of what a subtitle row costs or lacks, if anything. */
function subtitleCostKey(
  row: SubtitleRow,
):
  | "options.subtitles.refused"
  | "options.subtitles.burnIn"
  | "options.subtitles.unstyled"
  | undefined {
  if (row.refused) {
    return "options.subtitles.refused";
  }
  if (row.burnIn) {
    return "options.subtitles.burnIn";
  }
  return row.unstyled ? "options.subtitles.unstyled" : undefined;
}

/** PL-12: Off and the subtitle tracks; image tracks say that they are burned in. */
export function SubtitleSection({ view, actions }: OptionSectionProps) {
  const { t, i18n } = useTranslation(playerNamespace);
  const headingId = useId();
  const checked = [view.subtitlesOff, ...view.subtitles.map((row) => row.current)];
  const tabbable = tabbableIndex(checked);
  return (
    <section aria-labelledby={headingId}>
      <SectionHeading id={headingId}>{t("options.subtitles.heading")}</SectionHeading>
      <OptionMenu labelledBy={headingId}>
        <OptionItem
          checked={view.subtitlesOff}
          tabbable={tabbable === 0}
          onChoose={() => {
            actions.chooseSubtitle("");
          }}
        >
          <span>{t("options.subtitles.off")}</span>
        </OptionItem>
        {view.subtitles.map((row, position) => {
          const costKey = subtitleCostKey(row);
          return (
            <OptionItem
              key={row.id}
              checked={row.current}
              tabbable={tabbable === position + 1}
              onChoose={() => {
                actions.chooseSubtitle(row.id);
              }}
            >
              <span>
                {languageName(row.language, i18n.language) ??
                  t("options.subtitles.unknownLanguage")}
              </span>
              {row.title === undefined ? null : (
                <span className={secondaryLineClass}>{row.title}</span>
              )}
              <SubtitleBadges row={row} />
              {costKey === undefined ? null : (
                <span className={secondaryLineClass}>{t(costKey)}</span>
              )}
            </OptionItem>
          );
        })}
      </OptionMenu>
      <p className={hintClass}>
        {view.subtitles.length === 0 ? t("options.subtitles.none") : t("options.subtitles.instant")}
      </p>
    </section>
  );
}

/** PL-14, PL-15: whether the next item starts by itself, and the playback speed. */
export function PlaybackSection({ view, actions }: OptionSectionProps) {
  const { t } = useTranslation(playerNamespace);
  const headingId = useId();
  const autoPlayId = useId();
  return (
    <section aria-labelledby={headingId}>
      <SectionHeading id={headingId}>{t("options.playback.heading")}</SectionHeading>
      <div className="flex flex-col gap-3">
        <label htmlFor={autoPlayId} className="flex items-center gap-2 text-sm text-text">
          <input
            id={autoPlayId}
            type="checkbox"
            checked={view.autoPlay}
            onChange={(event) => {
              actions.setAutoPlay(event.target.checked);
            }}
            className="size-4 accent-accent"
          />
          {t("options.autoPlay.label")}
        </label>
        <SpeedControl value={view.speed} onChange={actions.setSpeed} />
      </div>
      <p className={hintClass}>{t("options.playback.device")}</p>
    </section>
  );
}

/** PL-13: the size of the subtitles the player draws; locked while a subtitle is burned in. */
export function SizeSection({ view, actions }: OptionSectionProps) {
  const { t } = useTranslation(playerNamespace);
  const headingId = useId();
  const hintId = useId();
  const locked = view.sizeLockedBy !== undefined;
  const options = subtitleSizes.map((size) => ({ id: size, label: t(`options.size.${size}`) }));
  return (
    <section aria-labelledby={headingId}>
      <SectionHeading id={headingId}>{t("options.size.heading")}</SectionHeading>
      <fieldset disabled={locked} aria-describedby={hintId} className="border-0 p-0">
        <SegmentedControl
          label={t("options.size.heading")}
          options={options}
          value={view.subtitleSize}
          onChange={(size) => {
            if (isSubtitleSize(size)) {
              actions.setSubtitleSize(size);
            }
          }}
        />
      </fieldset>
      <p id={hintId} className={hintClass}>
        {locked ? t("options.size.burnIn") : t("options.size.device")}
      </p>
    </section>
  );
}

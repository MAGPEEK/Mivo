import { belowBreakpointQuery } from "@mivo/ui-tokens";
import { useId, useRef, useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";

import { CloseIcon, Tabs, useMediaQuery } from "../../../shared/ui";
import { useModalFocus } from "../../../shared/ui/useModalFocus";
import type { PlayerActions, PlayerView } from "../hooks/usePlayer";
import { playerNamespace } from "../locales";
import {
  AudioSection,
  QualitySection,
  SizeSection,
  SubtitleSection,
  type OptionSectionProps,
} from "./OptionSections";
import { iconButtonClass } from "./playerStyles";
import { modeBadgeClasses } from "./StreamInfoOverlay";

/** Props of {@link PlaybackOptionsPanel}. */
export interface PlaybackOptionsPanelProps {
  readonly view: PlayerView;
  readonly actions: PlayerActions;
}

const narrowScreenQuery = belowBreakpointQuery("md");

type SectionId = "quality" | "audio" | "subtitles" | "size";

const sections: Readonly<Record<SectionId, (props: OptionSectionProps) => ReactElement>> = {
  quality: QualitySection,
  audio: AudioSection,
  subtitles: SubtitleSection,
  size: SizeSection,
};

/** Tab order of the phone sheet; the panel opens on audio, the most used section. */
const tabOrder: readonly SectionId[] = ["quality", "audio", "subtitles", "size"];
const startSection: SectionId = "audio";

function isSectionId(id: string): id is SectionId {
  return tabOrder.some((candidate) => candidate === id);
}

/** The current mode and the way to the stream-info overlay, at the foot of the panel. */
function PanelFooter({ view, actions }: PlaybackOptionsPanelProps) {
  const { t } = useTranslation(playerNamespace);
  const mode = view.streamInfo?.mode;
  return (
    <footer className="mt-4 flex items-center gap-3 border-t border-divider pt-3">
      {mode === undefined ? null : (
        <span
          className={`inline-flex rounded-sm px-2 py-0.5 text-sm font-semibold text-background ${modeBadgeClasses[mode.colorToken]}`}
        >
          {t(mode.key)}
        </span>
      )}
      <button
        type="button"
        onClick={() => {
          actions.options.close();
          actions.toggleStreamInfo();
        }}
        className="ms-auto rounded-md bg-surface px-3 py-1 text-sm hover:bg-divider"
      >
        {t("options.streamInfo")}
      </button>
    </footer>
  );
}

/** The title and the close button. */
function PanelHeader({
  titleId,
  onClose,
}: {
  readonly titleId: string;
  readonly onClose: () => void;
}) {
  const { t } = useTranslation(playerNamespace);
  return (
    <header className="mb-3 flex items-center justify-between gap-3">
      <h2 id={titleId} className="text-base font-semibold text-text">
        {t("options.title")}
      </h2>
      <button
        type="button"
        onClick={onClose}
        aria-label={t("options.close")}
        className={iconButtonClass}
      >
        <CloseIcon />
      </button>
    </header>
  );
}

/** All four sections side by side, or on narrow screens one at a time behind tabs. */
function PanelSections({
  narrow,
  ...sectionProps
}: OptionSectionProps & { readonly narrow: boolean }) {
  const { t } = useTranslation(playerNamespace);
  const [section, setSection] = useState<SectionId>(startSection);
  const Selected = sections[section];
  if (!narrow) {
    return (
      <div className="grid gap-6 md:grid-cols-4">
        {tabOrder.map((id) => {
          const Section = sections[id];
          return <Section key={id} {...sectionProps} />;
        })}
      </div>
    );
  }
  return (
    <Tabs
      label={t("options.tabs")}
      tabs={tabOrder.map((id) => ({ id, label: t(`options.tab.${id}`) }))}
      value={section}
      onChange={(id) => {
        if (isSectionId(id)) {
          setSection(id);
        }
      }}
    >
      <Selected {...sectionProps} />
    </Tabs>
  );
}

/**
 * The playback options (PL-10 … PL-13): quality, audio, subtitles and subtitle size, over the
 * lower part of the video; a bottom sheet with one tab per section on narrow screens. It is a
 * modal dialog: focus starts on the audio track playing, stays inside, and returns to the
 * options button on Escape, the close button or a press on the video. The panel stays open
 * after a choice, so its effect is visible.
 */
export function PlaybackOptionsPanel({ view, actions }: PlaybackOptionsPanelProps) {
  const titleId = useId();
  const dialogRef = useRef<HTMLDivElement>(null);
  const narrow = useMediaQuery(narrowScreenQuery);
  useModalFocus(dialogRef, actions.options.close);
  const placement = narrow
    ? "inset-x-0 bottom-0 max-h-[85dvh] rounded-t-xl border-t"
    : "inset-x-4 bottom-28 mx-auto max-h-[calc(100%-8rem)] max-w-5xl rounded-lg border";
  return (
    <>
      {/* The press on the video is a pointer convenience; keyboard users have Escape. */}
      {/* eslint-disable-next-line jsx-a11y/no-static-element-interactions */}
      <div
        data-testid="options-scrim"
        className="absolute inset-0 z-10"
        onMouseDown={actions.options.close}
      />
      {/* Keys stay in the dialog: the player's shortcuts (arrows seek, letters toggle) must not
          fire while a list is being operated. */}
      {/* eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions */}
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        onKeyDown={(event) => {
          event.stopPropagation();
        }}
        className={`absolute z-20 flex flex-col overflow-y-auto border-divider bg-surface-raised p-4 shadow-overlay ${placement}`}
      >
        <PanelHeader titleId={titleId} onClose={actions.options.close} />
        <PanelSections narrow={narrow} view={view.options} actions={actions.options} />
        <PanelFooter view={view} actions={actions} />
      </div>
    </>
  );
}

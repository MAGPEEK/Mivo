import type { ReasonText, StreamInfo, StreamInfoColorToken } from "@mivo/player-core";
import { useId, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { AlertIcon, CloseIcon } from "../../../shared/ui";
import { streamLineParts } from "../lib/streamLineText";
import { playerNamespace } from "../locales";
import { iconButtonClass } from "./playerStyles";

/** Props of {@link StreamInfoOverlay}. */
export interface StreamInfoOverlayProps {
  /** What to show; `undefined` before the first session. */
  readonly info: StreamInfo | undefined;
  readonly regionRef: RefObject<HTMLElement | null>;
  readonly headingRef: RefObject<HTMLHeadingElement | null>;
  readonly onClose: () => void;
}

// Status colours of ADR 0011 by player-core's token name; the compile-time check in
// apiTypeChecks.ts keeps the names in ui-tokens.
const modeBadgeClasses: Readonly<Record<StreamInfoColorToken, string>> = {
  directPlay: "bg-direct-play",
  directStream: "bg-direct-stream",
  transcode: "bg-transcode",
  textMuted: "bg-text-muted",
};

/**
 * The stream-info overlay (plan screen 4): mode, why, what happens to each stream, container,
 * encoder and source bit rate, as a labelled region beside the video.
 */
export function StreamInfoOverlay({
  info,
  regionRef,
  headingRef,
  onClose,
}: StreamInfoOverlayProps) {
  const { t } = useTranslation(playerNamespace);
  const headingId = useId();
  return (
    <section
      ref={regionRef}
      aria-labelledby={headingId}
      className="absolute top-20 right-4 max-h-[calc(100%-12rem)] w-96 max-w-[calc(100%-2rem)] overflow-y-auto rounded-lg bg-surface/95 p-4 shadow-overlay"
    >
      <div className="mb-3 flex items-center justify-between gap-2">
        <h2 ref={headingRef} id={headingId} tabIndex={-1} className="text-lg font-semibold">
          {t("overlay.heading")}
        </h2>
        <button
          type="button"
          onClick={onClose}
          aria-label={t("overlay.close")}
          className={iconButtonClass}
        >
          <CloseIcon />
        </button>
      </div>
      {info === undefined ? (
        <p className="text-text-muted">{t("overlay.noSession")}</p>
      ) : (
        <StreamInfoDetails info={info} />
      )}
    </section>
  );
}

function StreamInfoDetails({ info }: { readonly info: StreamInfo }) {
  const { t } = useTranslation(playerNamespace);
  const { mode, container, encoder, sourceBitRate } = info;
  return (
    <div className="flex flex-col gap-4 text-sm">
      <p>
        <span
          className={`inline-flex rounded-sm px-2 py-0.5 font-semibold text-background ${modeBadgeClasses[mode.colorToken]}`}
        >
          {t(mode.key)}
        </span>
      </p>
      <ReasonList heading={t("overlay.reasons")} reasons={info.reasons} />
      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5">
        <InfoRow
          label={t("overlay.container")}
          value={
            container.changed
              ? t("overlay.containerChange", { source: container.source, target: container.target })
              : container.target
          }
        />
        {info.streams.map((line) => (
          <InfoRow
            key={`${line.kind}-${String(line.sourceIndex)}`}
            label={t(line.kindKey)}
            value={[...streamLineParts(line), t(line.actionKey)].join(" · ")}
          />
        ))}
        {encoder === undefined ? null : (
          <InfoRow label={t("overlay.encoder")} value={t(encoder.key, encoder.params)} />
        )}
        {sourceBitRate === undefined ? null : (
          <InfoRow
            label={t("overlay.sourceBitRate")}
            value={t(sourceBitRate.key, sourceBitRate.params)}
          />
        )}
      </dl>
      <ReasonList heading={t("overlay.warnings")} reasons={info.warnings} warning />
    </div>
  );
}

function InfoRow({ label, value }: { readonly label: string; readonly value: string }) {
  return (
    <div className="contents">
      <dt className="text-text-muted">{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}

interface ReasonListProps {
  readonly heading: string;
  readonly reasons: readonly ReasonText[];
  /** Marks each entry with an alert icon in the transcode colour. */
  readonly warning?: boolean;
}

function ReasonList({ heading, reasons, warning = false }: ReasonListProps) {
  const { t } = useTranslation(playerNamespace);
  const headingId = useId();
  if (reasons.length === 0) {
    return null;
  }
  return (
    <div>
      <h3 id={headingId} className="mb-1 font-semibold text-text-muted">
        {heading}
      </h3>
      <ul aria-labelledby={headingId} className="flex flex-col gap-1">
        {reasons.map((reason) => (
          <li key={`${reason.scope}-${reason.code}`} className="flex items-start gap-2">
            {warning ? <AlertIcon className="mt-0.5 size-4 shrink-0 text-transcode" /> : null}
            {/* Through `replace`: params with an index signature make i18next's types read the
                key's underscores as a context suffix. */}
            <span>{t(reason.key, { replace: reason.params })}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

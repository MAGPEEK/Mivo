import type {
  AudioTrack,
  ReasonText,
  StreamInfo,
  StreamInfoColorToken,
  SubtitleInfo,
  TranscodeSpeedInfo,
} from "@mivo/player-core";
import { useId, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { AlertIcon, CloseIcon } from "../../../shared/ui";
import type { NetworkStats } from "../hooks/useNetworkStats";
import { reasonHintKey } from "../lib/reasonHints";
import { streamLineParts } from "../lib/streamLineText";
import { playerNamespace } from "../locales";
import { iconButtonClass } from "./playerStyles";

/** Props of {@link StreamInfoOverlay}. */
export interface StreamInfoOverlayProps {
  /** What to show; `undefined` before the first session. */
  readonly info: StreamInfo | undefined;
  /** Buffer ahead and throughput; unknown values read "unknown". */
  readonly networkStats: NetworkStats;
  /** Audio tracks of the file other than the one that plays; hints to switch need one. */
  readonly alternativeAudio: readonly AudioTrack[];
  readonly regionRef: RefObject<HTMLElement | null>;
  readonly headingRef: RefObject<HTMLHeadingElement | null>;
  readonly onClose: () => void;
}

const bytesPerMegabyte = 1_000_000;

// Status colours of ADR 0011 by player-core's token name; the compile-time check in
// apiTypeChecks.ts keeps the names in ui-tokens.
export const modeBadgeClasses: Readonly<Record<StreamInfoColorToken, string>> = {
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
  networkStats,
  alternativeAudio,
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
        <StreamInfoDetails
          info={info}
          networkStats={networkStats}
          alternativeAudio={alternativeAudio}
        />
      )}
    </section>
  );
}

function StreamInfoDetails({
  info,
  networkStats,
  alternativeAudio,
}: {
  readonly info: StreamInfo;
  readonly networkStats: NetworkStats;
  readonly alternativeAudio: readonly AudioTrack[];
}) {
  const { t } = useTranslation(playerNamespace);
  const { mode } = info;
  return (
    <div className="flex flex-col gap-4 text-sm">
      <p>
        <span
          className={`inline-flex rounded-sm px-2 py-0.5 font-semibold text-background ${modeBadgeClasses[mode.colorToken]}`}
        >
          {t(mode.key)}
        </span>
      </p>
      <ReasonList
        heading={t("overlay.reasons")}
        reasons={info.reasons}
        alternativeAudio={alternativeAudio}
      />
      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5">
        <FactRows info={info} />
        <NetworkRows stats={networkStats} />
      </dl>
      <ReasonList
        heading={t("overlay.warnings")}
        reasons={info.warnings}
        alternativeAudio={alternativeAudio}
        warning
      />
    </div>
  );
}

/** Buffer ahead and throughput as description rows; "unknown" until measured. */
function NetworkRows({ stats }: { readonly stats: NetworkStats }) {
  const { t } = useTranslation(playerNamespace);
  const { bufferAheadSeconds, throughputBytesPerSecond } = stats;
  const rows = [
    {
      key: "bufferAhead",
      label: t("overlay.bufferAhead"),
      value:
        bufferAheadSeconds === undefined
          ? t("overlay.unknown")
          : t("overlay.bufferAheadValue", { value: bufferAheadSeconds }),
    },
    {
      key: "throughput",
      label: t("overlay.throughput"),
      value:
        throughputBytesPerSecond === undefined
          ? t("overlay.unknown")
          : t("overlay.throughputValue", { value: throughputBytesPerSecond / bytesPerMegabyte }),
    },
  ];
  return rows.map((row) => <InfoRow key={row.key} label={row.label} value={row.value} />);
}

/** Container, one row per stream, encoder and source bit rate. */
function FactRows({ info }: { readonly info: StreamInfo }) {
  const { t } = useTranslation(playerNamespace);
  const { container, encoder, sourceBitRate } = info;
  return (
    <>
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
      <InfoRow
        label={t("overlay.outputBitRate")}
        value={
          info.outputBitRate === undefined
            ? t("overlay.unknown")
            : t(info.outputBitRate.key, info.outputBitRate.params)
        }
      />
      <SubtitleRow subtitle={info.subtitle} />
      {info.mode.id === "direct_play" ? null : <SpeedRow speed={info.speed} />}
    </>
  );
}

/** The subtitle mode with the selected track, e.g. "Rendered in the player · eng · SRT". */
function SubtitleRow({ subtitle }: { readonly subtitle: SubtitleInfo }) {
  const { t } = useTranslation(playerNamespace);
  const { track } = subtitle;
  const trackText =
    track === undefined
      ? undefined
      : t("overlay.subtitleTrack", {
          language: track.language ?? t("overlay.unknown"),
          format: track.format,
        });
  const value = [t(subtitle.key), trackText].filter((part) => part !== undefined).join(" · ");
  return <InfoRow label={t("overlay.subtitles")} value={value} />;
}

/** Transcode speed; below real time it takes the warning colour and says why. */
function SpeedRow({ speed }: { readonly speed: TranscodeSpeedInfo | undefined }) {
  const { t } = useTranslation(playerNamespace);
  return (
    <div className="contents">
      <dt className="text-text-muted">{t("overlay.speed")}</dt>
      <dd className={speed?.slow === true ? "text-transcode" : undefined}>
        {speed === undefined
          ? t("overlay.unknown")
          : t("overlay.speedValue", { value: speed.value })}
        {speed?.slow === true ? (
          <span className="flex items-center gap-1">
            <AlertIcon className="size-4 shrink-0" />
            {t("overlay.speedSlow")}
          </span>
        ) : null}
      </dd>
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
  readonly alternativeAudio: readonly AudioTrack[];
  /** Marks each entry with an alert icon in the transcode colour. */
  readonly warning?: boolean;
}

function ReasonList({ heading, reasons, alternativeAudio, warning = false }: ReasonListProps) {
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
            <span>
              {t(reason.key, { replace: reason.params })}
              <ReasonHint code={reason.code} alternativeAudio={alternativeAudio} />
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

function ReasonHint({
  code,
  alternativeAudio,
}: {
  readonly code: string;
  readonly alternativeAudio: readonly AudioTrack[];
}) {
  const { t } = useTranslation(playerNamespace);
  const key = reasonHintKey(code, alternativeAudio);
  if (key === undefined) {
    return null;
  }
  // The space keeps the sentences apart in the text a screen reader reads.
  return (
    <>
      {" "}
      <span className="block text-text-muted">{t(key)}</span>
    </>
  );
}

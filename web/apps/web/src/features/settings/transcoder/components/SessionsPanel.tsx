import { useId } from "react";
import { useTranslation } from "react-i18next";

import { ConfirmDialog, Skeleton, secondaryButtonClass } from "../../../../shared/ui";
import { settingsNamespace } from "../../locales";
import type { SessionSummary } from "../api/transcoderApi";
import {
  useTranscodeSessions,
  type SessionStop,
  type SessionsLoad,
} from "../hooks/useTranscodeSessions";
import { bitRateParts, isTooSlow, speedValue } from "../lib/transcoderView";

// Literal keys, so a misspelled one fails type checking; values not listed read "Unknown".
const modeKeys = {
  direct_play: "transcoder.sessions.mode.direct_play",
  remux: "transcoder.sessions.mode.remux",
  transcode: "transcoder.sessions.mode.transcode",
} as const;

const cellClass = "px-3 py-2 text-left align-top";

function EncoderText({ session }: { readonly session: SessionSummary }) {
  const { t } = useTranslation(settingsNamespace);
  if (session.mode === "direct_play" || session.mode === "remux") {
    return <>{t("transcoder.sessions.encoderCopy")}</>;
  }
  if (session.videoEncoder === undefined) {
    return <>{t("transcoder.sessions.unknownValue")}</>;
  }
  const encoder = session.videoEncoder;
  return (
    <>
      {session.hardware
        ? t("transcoder.sessions.encoderHardware", { encoder })
        : t("transcoder.sessions.encoderSoftware", { encoder })}
    </>
  );
}

function BitRateText({ session }: { readonly session: SessionSummary }) {
  const { t } = useTranslation(settingsNamespace);
  const parts = bitRateParts(session.outputBitRate);
  if (parts === null) {
    return <>{t("transcoder.sessions.unknownValue")}</>;
  }
  return parts.unit === "mbit" ? (
    <>{t("transcoder.sessions.bitRate.mbit", { value: parts.value })}</>
  ) : (
    <>{t("transcoder.sessions.bitRate.kbit", { value: parts.value })}</>
  );
}

function SpeedText({ session }: { readonly session: SessionSummary }) {
  const { t } = useTranslation(settingsNamespace);
  const speed = speedValue(session.transcodeSpeed);
  return (
    <>
      {speed === null
        ? t("transcoder.sessions.unknownValue")
        : t("transcoder.sessions.speed", { value: speed })}
      {isTooSlow(session) && (
        <span className="ml-2 text-transcode">{t("transcoder.sessions.tooSlow")}</span>
      )}
      {session.paused === true && (
        <span className="ml-2 text-text-muted">{t("transcoder.sessions.paused")}</span>
      )}
    </>
  );
}

interface SessionRowProps {
  readonly session: SessionSummary;
  readonly onStop: (session: SessionSummary) => void;
}

function SessionRow({ session, onStop }: SessionRowProps) {
  const { t } = useTranslation(settingsNamespace);
  const modeKey = Object.hasOwn(modeKeys, session.mode)
    ? modeKeys[session.mode]
    : "transcoder.sessions.mode.unknown";
  return (
    <tr className="border-t border-divider">
      {/* Title and client name come from the catalog and the client; shown as text only. */}
      <th scope="row" className={`${cellClass} font-medium`}>
        {session.title}
      </th>
      <td className={cellClass}>{session.clientName}</td>
      <td className={cellClass}>{t(modeKey)}</td>
      <td className={cellClass}>
        <EncoderText session={session} />
      </td>
      <td className={cellClass}>
        <BitRateText session={session} />
      </td>
      <td className={cellClass}>
        <SpeedText session={session} />
      </td>
      <td className={cellClass}>
        <button
          type="button"
          aria-label={t("transcoder.sessions.stopLabel", { title: session.title })}
          onClick={() => {
            onStop(session);
          }}
          className={secondaryButtonClass}
        >
          {t("transcoder.sessions.stop")}
        </button>
      </td>
    </tr>
  );
}

const columns = [
  { key: "title", label: "transcoder.sessions.column.title" },
  { key: "client", label: "transcoder.sessions.column.client" },
  { key: "mode", label: "transcoder.sessions.column.mode" },
  { key: "encoder", label: "transcoder.sessions.column.encoder" },
  { key: "bitRate", label: "transcoder.sessions.column.bitRate" },
  { key: "speed", label: "transcoder.sessions.column.speed" },
  { key: "actions", label: "transcoder.sessions.column.actions" },
] as const;

function StopDialog({ stop }: { readonly stop: SessionStop }) {
  const { t } = useTranslation(settingsNamespace);
  if (stop.target === null) {
    return null;
  }
  let error: string | undefined;
  if (stop.failure === "forbidden") {
    error = t("transcoder.forbidden");
  } else if (stop.failure === "failed") {
    error = t("transcoder.sessions.stopFailed");
  }
  return (
    <ConfirmDialog
      title={t("transcoder.sessions.stopDialog.title", { title: stop.target.title })}
      description={t("transcoder.sessions.stopDialog.description", {
        client: stop.target.clientName,
      })}
      confirmLabel={t("transcoder.sessions.stop")}
      cancelLabel={t("transcoder.sessions.stopDialog.cancel")}
      onConfirm={stop.confirm}
      onCancel={stop.cancel}
      pending={stop.pending}
      error={error}
    />
  );
}

function SessionTable({ load }: { readonly load: Extract<SessionsLoad, { state: "ready" }> }) {
  const { t } = useTranslation(settingsNamespace);
  if (load.sessions.length === 0) {
    return (
      <>
        <p className="text-text-muted">{t("transcoder.sessions.empty")}</p>
        <StopDialog stop={load.stop} />
      </>
    );
  }
  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse">
        <thead>
          <tr className="text-text-muted">
            {columns.map((column) => (
              <th key={column.key} scope="col" className={`${cellClass} font-medium`}>
                {t(column.label)}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {load.sessions.map((session) => (
            <SessionRow key={session.id} session={session} onStop={load.stop.ask} />
          ))}
        </tbody>
      </table>
      <StopDialog stop={load.stop} />
    </div>
  );
}

function SessionsContent({ load }: { readonly load: SessionsLoad }) {
  const { t } = useTranslation(settingsNamespace);
  switch (load.state) {
    case "loading":
      return <Skeleton label={t("transcoder.sessions.loading")} className="h-5 w-48" />;
    case "forbidden":
      // The status panel next to this one already shows the management-guard hint.
      return null;
    case "error":
      return (
        <div role="alert" className="flex flex-col items-start gap-3">
          <p>{t("transcoder.sessions.loadFailed")}</p>
          <button type="button" onClick={load.retry} className={secondaryButtonClass}>
            {t("transcoder.retry")}
          </button>
        </div>
      );
    case "ready":
      return <SessionTable load={load} />;
  }
}

/** The running sessions with a stop button each, refreshed while the page is visible. */
export function SessionsPanel() {
  const { t } = useTranslation(settingsNamespace);
  const headingId = useId();
  const load = useTranscodeSessions();
  if (load.state === "forbidden") {
    return null;
  }
  return (
    <section
      aria-labelledby={headingId}
      className="rounded-lg border border-divider bg-surface p-4 text-sm"
    >
      <h2 id={headingId} className="mb-3 text-base font-semibold">
        {t("transcoder.sessions.heading")}
      </h2>
      <SessionsContent load={load} />
    </section>
  );
}

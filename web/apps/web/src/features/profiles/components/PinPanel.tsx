import type { ProfileSummary } from "@mivo/api-client";
import { useEffect, useId, useRef } from "react";
import { useTranslation } from "react-i18next";

import { secondaryButtonClass } from "../../../shared/ui";
import { formatCountdown } from "../../auth";
import type { PinEntry } from "../hooks/usePinEntry";
import { padIndexAttribute, usePinKeyboard } from "../hooks/usePinKeyboard";
import { padKeys, pinLength, type PadKey } from "../lib/pinPad";

/** Props of {@link PinPanel}. */
export interface PinPanelProps {
  readonly profile: ProfileSummary;
  readonly pin: PinEntry;
  readonly remember: boolean;
  readonly onRememberChange: (remember: boolean) => void;
  readonly onCancel: () => void;
}

const padButtonClass =
  "h-14 w-16 rounded-md border border-divider bg-surface-raised text-xl text-text hover:bg-divider aria-disabled:cursor-not-allowed aria-disabled:text-text-muted";

function Dots({ pin }: { readonly pin: PinEntry }) {
  const { t } = useTranslation("profiles");
  return (
    <>
      {/* The key restarts the shake animation for every wrong PIN. */}
      <div
        key={pin.wrongCount}
        aria-hidden="true"
        className={`my-3 flex justify-center gap-2.5 ${pin.wrongCount > 0 ? "motion-safe:animate-pin-shake" : ""}`}
      >
        {Array.from({ length: pinLength }, (_, index) => (
          <span
            key={index}
            className={`size-3.5 rounded-full border-2 ${index < pin.length ? "border-accent bg-accent" : "border-text-muted"}`}
          />
        ))}
      </div>
      <p role="status" className="sr-only">
        {t("pin.entered", { count: pin.length, total: pinLength })}
      </p>
    </>
  );
}

function PadButton({ padKey, index, pin }: { padKey: PadKey; index: number; pin: PinEntry }) {
  const { t } = useTranslation("profiles");
  const labels = { delete: t("pin.delete"), submit: t("pin.submit") } as const;
  const symbols = { delete: "⌫", submit: "OK" } as const;
  const isDigit = typeof padKey === "number";
  return (
    <button
      type="button"
      {...{ [padIndexAttribute]: index }}
      aria-label={isDigit ? undefined : labels[padKey]}
      aria-disabled={pin.blocked}
      onClick={() => {
        pin.press(padKey);
      }}
      className={padButtonClass}
    >
      {isDigit ? String(padKey) : <span aria-hidden="true">{symbols[padKey]}</span>}
    </button>
  );
}

function FailureMessage({ pin }: { readonly pin: PinEntry }) {
  const { t } = useTranslation("profiles");
  const { failure, lockout } = pin;
  if (failure?.kind === "locked" && lockout.throttled) {
    return (
      <>
        <p aria-hidden="true" className="text-sm text-accent">
          {t("pin.locked", { time: formatCountdown(lockout.secondsLeft) })}
        </p>
        {/* Announced once with the whole wait, not on every tick of the countdown. */}
        <p role="alert" className="sr-only">
          {t("pin.locked", { time: formatCountdown(lockout.totalSeconds) })}
        </p>
      </>
    );
  }
  const texts = {
    wrongPin: t("pin.wrong"),
    locked: t("pin.unlocked"),
    network: t("picker.network"),
    generic: t("picker.failed"),
  } as const;
  return (
    <p role="alert" className="text-sm text-accent empty:hidden">
      {failure === undefined ? "" : texts[failure.kind]}
    </p>
  );
}

/**
 * The PIN pad of a profile (US-02, US-03): four dots, a 3×4 pad usable by pointer, keyboard
 * and TV remote, the failure and lockout messages and "Remember on this device".
 */
export function PinPanel({ profile, pin, remember, onRememberChange, onCancel }: PinPanelProps) {
  const { t } = useTranslation("profiles");
  const headingId = useId();
  const rememberId = useId();
  const panelRef = useRef<HTMLElement>(null);
  usePinKeyboard(panelRef, pin.press);
  useEffect(() => {
    panelRef.current?.querySelector<HTMLElement>(`[${padIndexAttribute}]`)?.focus();
  }, []);
  return (
    <section ref={panelRef} aria-labelledby={headingId} className="mx-auto w-full max-w-xs">
      <h2 id={headingId} className="text-lg font-semibold text-text">
        {t("pin.title", { name: profile.name })}
      </h2>
      <Dots pin={pin} />
      <div
        role="group"
        aria-label={t("pin.padLabel")}
        className="mx-auto my-3 grid w-fit grid-cols-3 gap-2"
      >
        {padKeys.map((padKey, index) => (
          <PadButton key={String(padKey)} padKey={padKey} index={index} pin={pin} />
        ))}
      </div>
      <FailureMessage pin={pin} />
      <div className="mt-3 flex items-center justify-center gap-2 text-sm text-text">
        <input
          id={rememberId}
          type="checkbox"
          checked={remember}
          onChange={(event) => {
            onRememberChange(event.target.checked);
          }}
          className="size-4 accent-accent"
        />
        <label htmlFor={rememberId}>{t("pin.remember")}</label>
      </div>
      <p className="mt-1 text-xs text-text-muted">{t("pin.rememberHint")}</p>
      <p className="mt-1 text-xs text-text-muted">{t("pin.attemptsHint")}</p>
      <button type="button" onClick={onCancel} className={`mt-4 ${secondaryButtonClass}`}>
        {t("pin.cancel")}
      </button>
    </section>
  );
}

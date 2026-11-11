import { useId, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { secondaryButtonClass } from "../../../../shared/ui";
import { inputClass } from "../../components/TextInput";
import { usersNamespace } from "../../locales";
import { useFocusOnChange } from "../hooks/useFocusOnChange";
import { pinLength, type PinDraft } from "../lib/pin";

/** Props of {@link PinField}. */
export interface PinFieldProps {
  readonly pin: PinDraft;
  /** True when the profile has a stored PIN: the field then offers change and remove. */
  readonly hasPin: boolean;
  readonly error?: string | undefined;
  readonly onType: (typed: string) => void;
  readonly onAction: (action: PinDraft["action"]) => void;
}

interface PinInputProps {
  readonly value: string;
  readonly labelKey: "profiles.dialog.pin.label" | "profiles.dialog.pin.newLabel";
  readonly error: string | undefined;
  readonly onType: (typed: string) => void;
  readonly inputRef: RefObject<HTMLInputElement | null>;
}

// The PIN is never shown: the server does not return it and the field masks what is typed.
function PinInput({ value, labelKey, error, onType, inputRef }: PinInputProps) {
  const { t } = useTranslation(usersNamespace);
  const id = useId();
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="text-sm font-medium text-text">
        {t(labelKey, { length: pinLength })}
      </label>
      <input
        ref={inputRef}
        id={id}
        type="password"
        inputMode="numeric"
        autoComplete="off"
        maxLength={pinLength}
        value={value}
        aria-invalid={error !== undefined}
        aria-describedby={`${id}-hint ${id}-error`}
        onChange={(event) => {
          onType(event.target.value);
        }}
        className={`${inputClass} max-w-40 tracking-widest`}
      />
      <p id={`${id}-hint`} className="text-xs text-text-muted">
        {t("profiles.dialog.pin.hint")}
      </p>
      <p id={`${id}-error`} role="alert" className="text-sm text-accent empty:hidden">
        {error}
      </p>
    </div>
  );
}

const pinActionClass = `${secondaryButtonClass} px-2 py-1 text-xs`;

interface PinActionsProps {
  readonly action: PinDraft["action"];
  readonly onAction: (action: PinDraft["action"]) => void;
  /** The first button; it takes the focus when the buttons replace the field or each other. */
  readonly firstRef: RefObject<HTMLButtonElement | null>;
}

// With a stored PIN untouched: "Change PIN" and "Remove PIN". Otherwise the way back.
function PinActions({ action, onAction, firstRef }: PinActionsProps) {
  const { t } = useTranslation(usersNamespace);
  const button = (target: PinDraft["action"], first: boolean) => (
    <button
      ref={first ? firstRef : undefined}
      type="button"
      onClick={() => {
        onAction(target);
      }}
      className={pinActionClass}
    >
      {t(`profiles.dialog.pin.${target}`)}
    </button>
  );
  if (action !== "keep") {
    return button("keep", action === "remove");
  }
  return (
    <div className="flex flex-wrap gap-2">
      {button("set", true)}
      {button("remove", false)}
    </div>
  );
}

/**
 * The PIN of a profile (US-02): an optional 4-digit field on a profile without PIN; on one
 * with a PIN "Change PIN" and "Remove PIN", each undone by "Keep the current PIN".
 */
export function PinField({ pin, hasPin, error, onType, onAction }: PinFieldProps) {
  const { t } = useTranslation(usersNamespace);
  // The controls replace each other in place, so the focus follows the change.
  const inputRef = useFocusOnChange<HTMLInputElement>(pin.action);
  const buttonRef = useFocusOnChange<HTMLButtonElement>(pin.action);
  const value = pin.action === "set" ? pin.value : "";
  if (!hasPin) {
    return (
      <div className="mb-4">
        <PinInput
          value={value}
          labelKey="profiles.dialog.pin.label"
          error={error}
          onType={onType}
          inputRef={inputRef}
        />
      </div>
    );
  }
  return (
    <fieldset className="mb-4 flex flex-col items-start gap-2">
      <legend className="mb-1 text-sm font-medium text-text">
        {t("profiles.dialog.pin.legend")}
      </legend>
      {pin.action === "keep" && <p className="text-sm">{t("profiles.dialog.pin.isSet")}</p>}
      {pin.action === "remove" && <p className="text-sm">{t("profiles.dialog.pin.willRemove")}</p>}
      {pin.action === "set" && (
        <PinInput
          value={value}
          labelKey="profiles.dialog.pin.newLabel"
          error={error}
          onType={onType}
          inputRef={inputRef}
        />
      )}
      <PinActions action={pin.action} onAction={onAction} firstRef={buttonRef} />
    </fieldset>
  );
}

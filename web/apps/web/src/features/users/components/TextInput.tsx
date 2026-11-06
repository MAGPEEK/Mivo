import { useEffect, useId, useRef, useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { usersNamespace } from "../locales";

/** Input look of the users feature; an invalid field gets the accent border. */
export const inputClass =
  "w-full rounded-md border border-divider bg-surface px-3 py-2 text-sm text-text aria-[invalid=true]:border-accent";

/** Props of {@link TextInput}. */
export interface TextInputProps {
  readonly label: string;
  readonly value: string;
  readonly onChange: (value: string) => void;
  readonly autoComplete: string;
  /** `password` adds a show/hide toggle. */
  readonly type?: "text" | "password";
  readonly hint?: string | undefined;
  /** Shown below the field and announced; the field is marked invalid while it is set. */
  readonly error?: string | undefined;
  readonly maxLength?: number;
  /** Takes the focus when the dialog opens. */
  readonly initialFocus?: boolean;
  /** Takes the focus when the field appears inside an open dialog. */
  readonly focusOnMount?: boolean;
}

function RevealButton(props: {
  readonly shown: boolean;
  readonly inputId: string;
  readonly onToggle: () => void;
}) {
  const { t } = useTranslation(usersNamespace);
  return (
    <button
      type="button"
      aria-label={props.shown ? t("users.dialog.hideLabel") : t("users.dialog.showLabel")}
      aria-controls={props.inputId}
      onClick={props.onToggle}
      className="absolute top-1/2 right-1 -translate-y-1/2 rounded-sm border border-divider bg-surface-raised px-2 py-1 text-xs text-text hover:border-text-muted"
    >
      {props.shown ? t("users.dialog.hide") : t("users.dialog.show")}
    </button>
  );
}

function useFocusOnMount(ref: RefObject<HTMLInputElement | null>, enabled: boolean): void {
  useEffect(() => {
    if (enabled) {
      ref.current?.focus();
    }
  }, [ref, enabled]);
}

/** A labelled single-line input with optional hint, error and password reveal. */
export function TextInput(props: TextInputProps) {
  const { label, value, onChange, hint, error, type = "text" } = props;
  const id = useId();
  const [revealed, setRevealed] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  useFocusOnMount(inputRef, props.focusOnMount === true);
  const hintId = `${id}-hint`;
  const errorId = `${id}-error`;
  const password = type === "password";
  return (
    <div className="mb-4 flex flex-col gap-1">
      <label htmlFor={id} className="text-sm font-medium text-text">
        {label}
      </label>
      <div className="relative">
        <input
          ref={inputRef}
          id={id}
          type={password && !revealed ? "password" : "text"}
          value={value}
          autoComplete={props.autoComplete}
          autoCapitalize="none"
          spellCheck={false}
          maxLength={props.maxLength}
          data-autofocus={props.initialFocus === true ? true : undefined}
          aria-invalid={error !== undefined}
          aria-describedby={hint === undefined ? errorId : `${hintId} ${errorId}`}
          onChange={(event) => {
            onChange(event.target.value);
          }}
          className={`${inputClass} ${password ? "pr-24" : ""}`}
        />
        {password && (
          <RevealButton
            shown={revealed}
            inputId={id}
            onToggle={() => {
              setRevealed((current) => !current);
            }}
          />
        )}
      </div>
      <FieldMessages hintId={hintId} hint={hint} errorId={errorId} error={error} />
    </div>
  );
}

interface FieldMessagesProps {
  readonly hintId: string;
  readonly hint: string | undefined;
  readonly errorId: string;
  readonly error: string | undefined;
}

// The error paragraph always exists, so a message that appears is announced by its live region.
function FieldMessages({ hintId, hint, errorId, error }: FieldMessagesProps) {
  return (
    <>
      <p id={hintId} className="text-xs text-text-muted empty:hidden">
        {hint}
      </p>
      <p id={errorId} role="alert" className="text-sm text-accent empty:hidden">
        {error}
      </p>
    </>
  );
}

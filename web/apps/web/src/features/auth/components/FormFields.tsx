import { useId, type ReactNode, type Ref } from "react";
import { useTranslation } from "react-i18next";

import { useReveal } from "../hooks/useReveal";

const inputClass =
  "w-full rounded-md border border-divider bg-background px-3 py-2 text-sm text-text aria-[invalid=true]:border-accent";

/** Props shared by {@link TextField} and {@link PasswordField}. */
interface FieldProps {
  readonly label: string;
  readonly value: string;
  readonly onChange: (value: string) => void;
  readonly autoComplete: string;
  /** The message shown below the field; the field is marked invalid while it is set. */
  readonly error?: string | undefined;
  readonly hint?: string | undefined;
  readonly inputRef?: Ref<HTMLInputElement> | undefined;
}

interface FieldFrameProps extends FieldProps {
  readonly type: "text" | "password";
  /** Extra classes of the input, e.g. room for the reveal button. */
  readonly inputClassName?: string;
  /** Rendered next to the input inside the same positioned box. */
  readonly renderAddon?: (inputId: string) => ReactNode;
}

function describedBy(...ids: (string | undefined)[]): string | undefined {
  const present = ids.filter((id) => id !== undefined);
  return present.length === 0 ? undefined : present.join(" ");
}

function FieldFrame(props: FieldFrameProps) {
  const { label, value, onChange, autoComplete, error, hint, inputRef, type } = props;
  const id = useId();
  const hintId = `${id}-hint`;
  const errorId = `${id}-error`;
  return (
    <div className="mb-4">
      <label htmlFor={id} className="mb-1 block text-sm font-medium text-text">
        {label}
      </label>
      <div className="relative">
        <input
          ref={inputRef}
          id={id}
          type={type}
          value={value}
          autoComplete={autoComplete}
          autoCapitalize="none"
          spellCheck={false}
          aria-invalid={error !== undefined}
          aria-describedby={describedBy(
            hint === undefined ? undefined : hintId,
            error === undefined ? undefined : errorId,
          )}
          onChange={(event) => {
            onChange(event.target.value);
          }}
          className={`${inputClass} ${props.inputClassName ?? ""}`}
        />
        {props.renderAddon?.(id)}
      </div>
      {hint !== undefined && (
        <p id={hintId} className="mt-1 text-xs text-text-muted">
          {hint}
        </p>
      )}
      {error !== undefined && (
        <p id={errorId} className="mt-1 text-sm text-accent">
          {error}
        </p>
      )}
    </div>
  );
}

/** A labelled single-line text input with optional hint and error. */
export function TextField(props: FieldProps) {
  return <FieldFrame {...props} type="text" />;
}

/** A labelled password input with a show/hide toggle, optional hint and error. */
export function PasswordField(props: FieldProps) {
  const { t } = useTranslation("auth");
  const reveal = useReveal();
  return (
    <FieldFrame
      {...props}
      type={reveal.shown ? "text" : "password"}
      inputClassName="pr-24"
      renderAddon={(inputId) => (
        <button
          type="button"
          aria-label={reveal.shown ? t("fields.hideLabel") : t("fields.showLabel")}
          aria-controls={inputId}
          onClick={reveal.toggle}
          className="absolute top-1/2 right-1 -translate-y-1/2 rounded-sm border border-divider bg-surface-raised px-2 py-1 text-xs text-text hover:border-text-muted"
        >
          {reveal.shown ? t("fields.hide") : t("fields.show")}
        </button>
      )}
    />
  );
}

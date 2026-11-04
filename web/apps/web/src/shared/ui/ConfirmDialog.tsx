import { useId, useRef } from "react";

import { primaryButtonClass, secondaryButtonClass } from "./buttonClasses";
import { useModalFocus } from "./useModalFocus";

/** Props of {@link ConfirmDialog}. */
export interface ConfirmDialogProps {
  readonly title: string;
  /** What happens when the user confirms. */
  readonly description: string;
  readonly confirmLabel: string;
  readonly cancelLabel: string;
  readonly onConfirm: () => void;
  /** Called by the cancel button and by Escape. */
  readonly onCancel: () => void;
  /** True while the confirmed action runs; the confirm button is then `aria-disabled`. */
  readonly pending?: boolean;
  /** Why the confirmed action failed; shown inside the dialog and announced. */
  readonly error?: string | undefined;
}

/**
 * Modal confirmation. Render it only while it is open. Focus starts on the cancel button, the
 * safe choice for a destructive action, and returns to the opener afterwards.
 */
export function ConfirmDialog({
  title,
  description,
  confirmLabel,
  cancelLabel,
  onConfirm,
  onCancel,
  pending = false,
  error,
}: ConfirmDialogProps) {
  const titleId = useId();
  const descriptionId = useId();
  const dialogRef = useRef<HTMLDivElement>(null);
  useModalFocus(dialogRef, onCancel);
  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-black/60 p-4">
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={descriptionId}
        className="w-full max-w-md rounded-lg border border-divider bg-surface-raised p-6 shadow-overlay"
      >
        <h2 id={titleId} className="text-lg font-semibold text-text">
          {title}
        </h2>
        <p id={descriptionId} className="mt-2 text-sm text-text-muted">
          {description}
        </p>
        <p role="alert" className="mt-3 text-sm text-accent empty:hidden">
          {error}
        </p>
        <div className="mt-6 flex justify-end gap-3">
          <button type="button" data-autofocus onClick={onCancel} className={secondaryButtonClass}>
            {cancelLabel}
          </button>
          <button
            type="button"
            aria-disabled={pending}
            onClick={pending ? undefined : onConfirm}
            className={primaryButtonClass}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}

import { useId, useRef, type ReactNode } from "react";

import { secondaryButtonClass } from "./buttonClasses";
import { useModalFocus } from "./useModalFocus";

/** Props of {@link ModalDialog}. */
export interface ModalDialogProps {
  readonly title: string;
  /** Accessible name of the close button in the header. */
  readonly closeLabel: string;
  /** Called by the close button and by Escape. */
  readonly onClose: () => void;
  readonly children: ReactNode;
  /** Action row below the content. */
  readonly footer?: ReactNode;
  /** `wide` fits forms with tabs and lists; `narrow` short questions. */
  readonly size?: "narrow" | "wide";
}

const sizeClasses = { narrow: "max-w-md", wide: "max-w-3xl" } as const;

/**
 * Modal dialog frame with title, close button, scrollable body and footer. Render it only while
 * it is open; focus handling comes from {@link useModalFocus}.
 */
export function ModalDialog({
  title,
  closeLabel,
  onClose,
  children,
  footer,
  size = "wide",
}: ModalDialogProps) {
  const titleId = useId();
  const dialogRef = useRef<HTMLDivElement>(null);
  useModalFocus(dialogRef, onClose);
  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-scrim/60 p-4">
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        className={`flex max-h-full w-full flex-col rounded-lg border border-divider bg-surface-raised shadow-overlay ${sizeClasses[size]}`}
      >
        <header className="flex items-center justify-between gap-3 border-b border-divider px-6 py-4">
          <h2 id={titleId} className="text-lg font-semibold text-text">
            {title}
          </h2>
          <button
            type="button"
            onClick={onClose}
            aria-label={closeLabel}
            className={`${secondaryButtonClass} px-2 py-1`}
          >
            <span aria-hidden="true">✕</span>
          </button>
        </header>
        <div className="min-h-0 flex-1 overflow-y-auto px-6 py-4">{children}</div>
        {footer !== undefined && (
          <footer className="flex flex-wrap items-center justify-between gap-3 border-t border-divider px-6 py-4">
            {footer}
          </footer>
        )}
      </div>
    </div>
  );
}

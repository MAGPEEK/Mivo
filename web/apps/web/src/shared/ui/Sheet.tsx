import { belowBreakpointQuery } from "@mivo/ui-tokens";
import { useId, useRef, type ReactNode } from "react";

import { secondaryButtonClass } from "./buttonClasses";
import { useMediaQuery } from "./useMediaQuery";
import { useModalFocus } from "./useModalFocus";
import { useScrollLock } from "./useScrollLock";

/** Props of {@link Sheet}. */
export interface SheetProps {
  readonly title: string;
  /** Accessible name of the close button in the header. */
  readonly closeLabel: string;
  /** Called by the close button, a press on the scrim and Escape. */
  readonly onClose: () => void;
  readonly children: ReactNode;
}

// Same width as the shell's narrow-screen switch (Tailwind `md`).
const narrowScreenQuery = belowBreakpointQuery("md");

/**
 * Modal surface that is a bottom sheet on narrow screens and a centred dialog elsewhere. Render
 * it only while it is open. It traps focus, closes on Escape or a press on the scrim, locks page
 * scrolling and slides in only when the user has not asked for reduced motion.
 */
export function Sheet({ title, closeLabel, onClose, children }: SheetProps) {
  const titleId = useId();
  const dialogRef = useRef<HTMLDivElement>(null);
  const narrow = useMediaQuery(narrowScreenQuery);
  useModalFocus(dialogRef, onClose);
  useScrollLock();
  const placement = narrow
    ? "max-h-[85dvh] rounded-t-xl border-t pb-[max(calc(var(--spacing)*4),env(safe-area-inset-bottom))] transition-transform duration-200 starting:translate-y-full motion-reduce:transition-none"
    : "max-h-full max-w-lg rounded-lg border";
  return (
    // The scrim press is a pointer convenience; keyboard users have Escape and the close button.
    // eslint-disable-next-line jsx-a11y/no-static-element-interactions
    <div
      data-testid="sheet-scrim"
      className={`fixed inset-0 z-50 grid bg-scrim/60 ${narrow ? "items-end" : "place-items-center p-4"}`}
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) {
          onClose();
        }
      }}
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        data-placement={narrow ? "bottom" : "center"}
        className={`flex w-full flex-col border-divider bg-surface-raised shadow-overlay ${placement}`}
      >
        <header className="flex items-center justify-between gap-3 border-b border-divider px-5 py-3">
          <h2 id={titleId} className="text-base font-semibold text-text">
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
        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">{children}</div>
      </div>
    </div>
  );
}

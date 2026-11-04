import { useEffect, type RefObject } from "react";

const focusableSelector =
  "button:not([disabled]), a[href], input:not([disabled]), select, textarea";

// Tab at either end of the dialog wraps to the other end.
function wrapTab(dialog: HTMLElement, event: KeyboardEvent): void {
  const focusable = [...dialog.querySelectorAll<HTMLElement>(focusableSelector)];
  const first = focusable[0];
  const last = focusable[focusable.length - 1];
  if (first === undefined || last === undefined) {
    return;
  }
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault();
    last.focus();
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault();
    first.focus();
  }
}

/**
 * Focus handling of a modal dialog: focus moves to the first element marked
 * `data-autofocus` (else the first focusable one) when it opens and returns to the opener when
 * it closes; Tab wraps around inside and Escape calls `onClose`. The key listener sits on the
 * dialog element itself, because a dialog role is not an interactive target for JSX handlers.
 */
export function useModalFocus(dialogRef: RefObject<HTMLElement | null>, onClose: () => void): void {
  useEffect(() => {
    const opener = document.activeElement;
    const dialog = dialogRef.current;
    const target =
      dialog?.querySelector<HTMLElement>("[data-autofocus]") ??
      dialog?.querySelector<HTMLElement>(focusableSelector);
    target?.focus();
    return () => {
      if (opener instanceof HTMLElement && opener.isConnected) {
        opener.focus();
      }
    };
  }, [dialogRef]);
  useEffect(() => {
    const dialog = dialogRef.current;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.stopPropagation();
        onClose();
      } else if (event.key === "Tab" && dialog !== null) {
        wrapTab(dialog, event);
      }
    };
    dialog?.addEventListener("keydown", onKeyDown);
    return () => {
      dialog?.removeEventListener("keydown", onKeyDown);
    };
  }, [dialogRef, onClose]);
}

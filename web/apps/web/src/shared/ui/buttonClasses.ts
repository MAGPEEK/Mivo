// Button looks of the library feature. Disabled styles key on both `disabled` and
// `aria-disabled`: buttons that must keep keyboard focus while busy use the latter.

/** Coral call to action, e.g. "Scan library" or "Try again". */
export const primaryButtonClass =
  "rounded-md bg-accent px-4 py-2 text-sm font-medium text-on-accent hover:bg-accent-hover disabled:cursor-not-allowed disabled:bg-surface-raised disabled:text-text-muted aria-disabled:cursor-not-allowed aria-disabled:bg-surface-raised aria-disabled:text-text-muted";

/** Quiet secondary action, e.g. "Load more". */
export const secondaryButtonClass =
  "rounded-md border border-divider bg-surface px-4 py-2 text-sm font-medium text-text hover:bg-surface-raised disabled:cursor-not-allowed disabled:text-text-muted aria-disabled:cursor-not-allowed aria-disabled:text-text-muted";

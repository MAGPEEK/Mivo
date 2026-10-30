/** Subtle shadows (ADR 0011). On a dark theme they separate layers rather than imitate light. */
export const shadows = {
  /** Resting card in a poster row. */
  card: "0 1px 2px rgb(0 0 0 / 0.3), 0 1px 3px rgb(0 0 0 / 0.2)",
  /** Card lifted on hover or focus. */
  cardHover: "0 8px 24px rgb(0 0 0 / 0.45)",
  /** Menus, dialogs and the player side panel. */
  overlay: "0 16px 48px rgb(0 0 0 / 0.55)",
} as const;

/** Name of a shadow token. */
export type ShadowToken = keyof typeof shadows;

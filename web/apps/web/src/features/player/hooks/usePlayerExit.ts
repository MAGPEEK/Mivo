import { useRouter } from "@tanstack/react-router";

/** Ways out of the player. */
export interface PlayerExit {
  /**
   * Back in the history when the player was opened from a page of the app, otherwise (a
   * shared link, a reload) to the item's page.
   */
  readonly exit: () => void;
  /** Always to the item's page, e.g. from an error a retry cannot fix. */
  readonly openItem: () => void;
}

/**
 * Leaves the player. Opening the item page replaces the player entry, so Back does not reopen
 * a player that cannot play.
 */
export function usePlayerExit(itemId: string): PlayerExit {
  const router = useRouter();
  const openItem = () => {
    void router.navigate({ to: "/items/$itemId", params: { itemId }, replace: true });
  };
  return {
    exit: () => {
      if (router.history.canGoBack()) {
        router.history.back();
      } else {
        openItem();
      }
    },
    openItem,
  };
}

import { useRouter } from "@tanstack/react-router";
import { useMemo } from "react";

import { readPlayChoice, type PlayChoice } from "../lib/playChoiceHandoff";

/**
 * The audio and subtitle pick the detail page handed over for `itemId`, read from the history
 * state when the item opens. The same object is returned until the item changes, so the
 * playback is not reopened by a re-render. A queue's next or previous item arrives without
 * state, and the choice names its item, so it never leaks to another one.
 */
export function useInitialPlayChoice(itemId: string): PlayChoice {
  const router = useRouter();
  return useMemo(() => readPlayChoice(router.state.location.state, itemId), [router, itemId]);
}

import { useMediaQuery } from "../../shared/ui";
import { phoneScreenQuery } from "./useNarrowScreen";

/** Whether the window is narrower than the `sm` breakpoint, where the bottom navigation shows. */
export function usePhoneScreen(): boolean {
  return useMediaQuery(phoneScreenQuery);
}

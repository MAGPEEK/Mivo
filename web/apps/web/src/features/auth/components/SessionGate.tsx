import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { useSessionGuard } from "../hooks/useSessionGuard";
import { useUnauthorizedHandler } from "../hooks/useUnauthorizedHandler";

/** Props of {@link SessionGate}. */
export interface SessionGateProps {
  readonly children: ReactNode;
}

/**
 * Wraps every page: shows it once the session allows it, sends the visitor to setup, sign-in
 * or the profile picker otherwise, and turns any later `401` into the sign-in page.
 */
export function SessionGate({ children }: SessionGateProps) {
  const { t } = useTranslation("auth");
  const { view } = useSessionGuard();
  useUnauthorizedHandler();
  if (view === "page") {
    return children;
  }
  return (
    <div role="status" className="grid min-h-dvh place-items-center text-sm text-text-muted">
      <span className="sr-only">{t("loading")}</span>
    </div>
  );
}

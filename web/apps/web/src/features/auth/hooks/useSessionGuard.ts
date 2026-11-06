import { useLocation, useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";

import { guardRedirect } from "../lib/guard";
import type { SessionStatus } from "../lib/sessionStatus";
import { useSession } from "./useSession";

/** What the gate renders: nothing while it redirects or loads, else the page. */
export type GateView = "loading" | "redirecting" | "page";

/**
 * Sends the visitor where the session says they belong (setup, sign-in, profile picker or back
 * to the page they came for) and tells the gate what to render meanwhile.
 */
export function useSessionGuard(): { readonly view: GateView; readonly status: SessionStatus } {
  const session = useSession();
  const location = useLocation();
  const navigate = useNavigate();
  const search: Readonly<Record<string, unknown>> = location.search;
  const redirect = guardRedirect(session, {
    pathname: location.pathname,
    href: location.href,
    next: search["next"],
  });
  useEffect(() => {
    if (redirect !== undefined) {
      void navigate({ href: redirect, replace: true });
    }
  }, [navigate, redirect]);
  if (redirect !== undefined) {
    return { view: "redirecting", status: session.status };
  }
  return { view: session.status === "loading" ? "loading" : "page", status: session.status };
}

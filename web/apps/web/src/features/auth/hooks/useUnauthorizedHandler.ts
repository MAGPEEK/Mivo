import { addResponseListener, type ObservedResponse } from "@mivo/api-client";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { useRouter, type HistoryState } from "@tanstack/react-router";
import { useEffect } from "react";

import { useApiClient } from "../../../shared/api";
import { meQueryKey } from "../api/authApi";
import { signInPath } from "../lib/authPaths";
import { authHref } from "../lib/authSearch";
import { returnTarget } from "../lib/guard";
import { mayAffectSession, unauthorizedAction } from "../lib/unauthorized";
import { cachedAccountName, resetSession } from "./sessionCache";

type AppRouter = ReturnType<typeof useRouter>;

/** History state of the sign-in page after a lost session: the name to prefill (AU-06). */
export interface SignInHistoryState {
  readonly signInName?: string;
}

async function problemCode(response: Response): Promise<string | undefined> {
  try {
    const body = (await response.clone().json()) as unknown;
    const code =
      typeof body === "object" && body !== null && "code" in body ? body.code : undefined;
    return typeof code === "string" ? code : undefined;
  } catch {
    return undefined;
  }
}

/** The services an answer handler acts on, and whether a sign-in redirect is under way. */
interface HandlerContext {
  readonly queryClient: QueryClient;
  readonly router: AppRouter;
  redirecting: boolean;
}

// Several requests fail at once when a session ends; only the first one redirects, so the
// prefilled name and the page to return to are taken before the cache is cleared.
async function sendToSignIn(context: HandlerContext): Promise<void> {
  const { queryClient, router } = context;
  const { location } = router.state;
  if (context.redirecting || location.pathname === signInPath) {
    return;
  }
  context.redirecting = true;
  const next = returnTarget({
    pathname: location.pathname,
    href: location.href,
    next: new URLSearchParams(location.searchStr).get("next") ?? undefined,
  });
  const name = cachedAccountName(queryClient);
  // Typed as both, so the router accepts the extra member as history state.
  const state: HistoryState & SignInHistoryState = name === undefined ? {} : { signInName: name };
  resetSession(queryClient);
  try {
    await router.navigate({
      href: authHref(signInPath, { next, reason: "expired" }),
      replace: true,
      state: () => state,
    });
  } finally {
    context.redirecting = false;
  }
}

async function handleAnswer(observed: ObservedResponse, context: HandlerContext): Promise<void> {
  const { method, schemaPath, response } = observed;
  if (!mayAffectSession(response.status)) {
    return;
  }
  const code = await problemCode(response);
  const { queryClient } = context;
  switch (unauthorizedAction({ method, schemaPath, status: response.status, code })) {
    case "signIn":
      await sendToSignIn(context);
      return;
    case "setup":
      // The guard sends the visitor to the setup once the session check says so.
      resetSession(queryClient);
      return;
    case "profiles":
      await queryClient.invalidateQueries({ queryKey: meQueryKey });
      return;
    case undefined:
      return;
  }
}

/**
 * Watches every API answer: a `401` clears the cached data and opens the sign-in page with the
 * current page as `next` ("Session expired", AU-06); a `403 profile_required` lets the guard
 * open the profile picker. Answers that belong to a form, such as `403 invalid_credentials`
 * when changing the password, are left to it.
 */
export function useUnauthorizedHandler(): void {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const router = useRouter();
  useEffect(() => {
    const context: HandlerContext = { queryClient, router, redirecting: false };
    return addResponseListener(client, (observed) => handleAnswer(observed, context));
  }, [client, queryClient, router]);
}

import type { Middleware } from "openapi-fetch";

import type { ApiClient } from "./client";

/** What a {@link ResponseListener} learns about every response the client receives. */
export interface ObservedResponse {
  /** HTTP method in upper case, e.g. `GET`. */
  readonly method: string;
  /** The contract path with its placeholders, e.g. `/api/v1/items/{itemId}`. */
  readonly schemaPath: string;
  /**
   * The response. Its body belongs to the caller of the request: read it only through
   * `response.clone()`.
   */
  readonly response: Response;
}

/** Observes responses; it cannot change them. A returned promise is awaited, a rejection ignored. */
export type ResponseListener = (observed: ObservedResponse) => void | Promise<void>;

/**
 * Calls `listener` for every HTTP response of `client`, before the caller sees it. The single
 * hook through which an app reacts to cross-cutting answers such as `401 unauthenticated`
 * (ADR 0032) without touching every request. Network failures produce no response and are not
 * observed. Returns a function that removes the listener again.
 */
export function addResponseListener(client: ApiClient, listener: ResponseListener): () => void {
  const middleware: Middleware = {
    async onResponse({ request, schemaPath, response }) {
      try {
        await listener({ method: request.method.toUpperCase(), schemaPath, response });
      } catch {
        // A failing listener must not turn a valid answer into an error for the caller.
      }
      return undefined;
    },
  };
  client.use(middleware);
  return () => {
    client.eject(middleware);
  };
}

import type { Me, Session, SetupState } from "@mivo/api-client";

import {
  createFakeServer,
  fakeMe,
  jsonResponse,
  problemResponse,
  type FakeRoute,
  type FakeServer,
} from "../../../test/fakeApi";

/** Where the fake server's session stands; tests change it between steps. */
export interface FakeAuthState {
  setupRequired: boolean;
  signedIn: boolean;
  /** The profile of the session; `null` makes the app ask for one. */
  currentProfile: Me["currentProfile"];
}

/** A fake server with accounts, plus its mutable state. */
export interface FakeAuthServer extends FakeServer {
  readonly state: FakeAuthState;
}

/** Options of {@link createFakeAuthServer}. */
export interface FakeAuthOptions {
  readonly initial?: Partial<FakeAuthState>;
  /** Routes before the auth routes, e.g. failures of sign-in or catalog requests. */
  readonly routes?: readonly FakeRoute[];
}

/** The account name and password the fake server accepts. */
export const fakeCredentials = { name: "Anna", password: "nordlicht-2026" } as const;

function currentMe(state: FakeAuthState): Me {
  return { ...fakeMe, currentProfile: state.currentProfile };
}

function session(state: FakeAuthState): Response {
  state.setupRequired = false;
  state.signedIn = true;
  const body: Session = { me: currentMe(state) };
  return jsonResponse(body);
}

function authRoutes(state: FakeAuthState): FakeRoute[] {
  return [
    {
      method: "GET",
      path: "/api/v1/setup",
      respond: () => jsonResponse({ required: state.setupRequired } satisfies SetupState),
    },
    {
      method: "GET",
      path: "/api/v1/me",
      respond: () => {
        if (state.setupRequired) {
          return problemResponse(401, "setup_required");
        }
        return state.signedIn
          ? jsonResponse(currentMe(state))
          : problemResponse(401, "unauthenticated");
      },
    },
    {
      method: "POST",
      path: "/api/v1/auth/session",
      respond: async ({ request }) => {
        const body = (await request.clone().json()) as {
          readonly name?: string;
          readonly password?: string;
        };
        return body.name === fakeCredentials.name && body.password === fakeCredentials.password
          ? session(state)
          : problemResponse(401, "invalid_credentials");
      },
    },
    { method: "POST", path: "/api/v1/setup", respond: () => session(state) },
    {
      method: "DELETE",
      path: "/api/v1/auth/session",
      respond: () => {
        state.signedIn = false;
        return new Response(null, { status: 204 });
      },
    },
  ];
}

/** Creates a fake server that answers setup, sign-in, sign-out and `GET /me` from its state. */
export function createFakeAuthServer(options: FakeAuthOptions = {}): FakeAuthServer {
  const state: FakeAuthState = {
    setupRequired: false,
    signedIn: true,
    currentProfile: fakeMe.currentProfile,
    ...options.initial,
  };
  const server = createFakeServer([...(options.routes ?? []), ...authRoutes(state)]);
  return { ...server, state };
}

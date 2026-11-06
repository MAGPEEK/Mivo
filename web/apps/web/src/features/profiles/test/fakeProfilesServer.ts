import type { Me, Problem, ProfileSummary } from "@mivo/api-client";

import {
  createFakeServer,
  fakeMe,
  jsonResponse,
  problemResponse,
  type FakeRoute,
  type FakeServer,
} from "../../../test/fakeApi";

/** The account owner's primary profile: admin, no PIN. */
export const annaProfile: ProfileSummary = {
  id: "6f5e4d3c-2b1a-4f0e-9d8c-7b6a5f4e3d2c",
  name: "Anna",
  avatarColor: "coral",
  hasPin: false,
  isPrimary: true,
  isAdmin: true,
};

/** A managed profile with the PIN {@link maxPin}. */
export const maxProfile: ProfileSummary = {
  id: "0b1c2d3e-4f50-4617-8a9b-0c1d2e3f4a5b",
  name: "Max",
  avatarColor: "blue",
  hasPin: true,
  isPrimary: false,
  isAdmin: false,
};

/** A managed profile without PIN. */
export const lenaProfile: ProfileSummary = {
  id: "9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d",
  name: "Lena",
  avatarColor: "green",
  hasPin: false,
  isPrimary: false,
  isAdmin: false,
};

/** The PIN of {@link maxProfile}. */
export const maxPin = "1234";

/** A profile selection the fake server received. */
export interface ReceivedSelection {
  readonly profileId: string;
  readonly pin?: string;
  readonly remember?: boolean;
}

/** Mutable state of the fake server; tests change it between steps. */
export interface FakeProfilesState {
  me: Me;
  /** Wrong PINs before the profile locks; then `429` with {@link lockoutSeconds}. */
  wrongPinsUntilLock: number;
  lockoutSeconds: number;
  readonly selections: ReceivedSelection[];
}

/** A fake server with an account, its profiles and the profile endpoints. */
export interface FakeProfilesServer extends FakeServer {
  readonly state: FakeProfilesState;
}

const defaultWrongPinsUntilLock = 5;
const defaultLockoutSeconds = 300;

function lockedResponse(seconds: number): Response {
  const problem: Problem & { readonly retryAfterSeconds: number } = {
    type: "about:blank",
    title: "profile_locked",
    status: 429,
    code: "profile_locked",
    retryAfterSeconds: seconds,
  };
  return jsonResponse(problem, problem.status, "application/problem+json");
}

function selectResponse(state: FakeProfilesState, selection: ReceivedSelection): Response {
  state.selections.push(selection);
  const profile = state.me.profiles.find((candidate) => candidate.id === selection.profileId);
  if (profile === undefined) {
    return problemResponse(404, "not_found");
  }
  if (profile.hasPin && selection.pin !== maxPin) {
    if (state.wrongPinsUntilLock <= 1) {
      return lockedResponse(state.lockoutSeconds);
    }
    state.wrongPinsUntilLock -= 1;
    return problemResponse(403, "pin_invalid");
  }
  state.me = { ...state.me, currentProfile: profile };
  return jsonResponse(state.me);
}

function profileRoutes(state: FakeProfilesState): FakeRoute[] {
  return [
    { method: "GET", path: "/api/v1/me", respond: () => jsonResponse(state.me) },
    {
      method: "PUT",
      path: "/api/v1/me/profile",
      respond: async ({ request }) =>
        selectResponse(state, (await request.clone().json()) as ReceivedSelection),
    },
    {
      method: "DELETE",
      path: "/api/v1/me/profile",
      respond: () => {
        state.me = { ...state.me, currentProfile: null };
        return new Response(null, { status: 204 });
      },
    },
    {
      method: "DELETE",
      path: "/api/v1/auth/session",
      respond: () => new Response(null, { status: 204 }),
    },
  ];
}

/** Options of {@link createFakeProfilesServer}. */
export interface FakeProfilesOptions {
  readonly currentProfile?: ProfileSummary | null;
  readonly profiles?: readonly ProfileSummary[];
  /** Routes before the profile routes, e.g. failures. */
  readonly routes?: readonly FakeRoute[];
}

/** Creates a fake server for Anna's account with the profiles Anna, Max (PIN) and Lena. */
export function createFakeProfilesServer(options: FakeProfilesOptions = {}): FakeProfilesServer {
  const state: FakeProfilesState = {
    me: {
      ...fakeMe,
      currentProfile: options.currentProfile === undefined ? null : options.currentProfile,
      profiles: [...(options.profiles ?? [annaProfile, lenaProfile, maxProfile])],
    },
    wrongPinsUntilLock: defaultWrongPinsUntilLock,
    lockoutSeconds: defaultLockoutSeconds,
    selections: [],
  };
  const server = createFakeServer([...(options.routes ?? []), ...profileRoutes(state)]);
  return { ...server, state };
}

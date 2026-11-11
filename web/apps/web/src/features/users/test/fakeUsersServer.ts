import type {
  Library,
  Me,
  Profile,
  ProfileCreate,
  ProfileUpdate,
  User,
  UserCreate,
  UserUpdate,
} from "@mivo/api-client";

import {
  createFakeServer,
  fakeMe,
  jsonResponse,
  problemResponse,
  type FakeRoute,
  type FakeServer,
} from "../../../test/fakeApi";
import { fakeLibrary } from "../../../test/libraryFixtures";

const createdStatus = 201;
const noContentStatus = 204;

/** Libraries of the fake server. */
export const movies: Library = fakeLibrary({});
export const shows: Library = fakeLibrary({
  id: "9c8b7a6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d",
  name: "Shows",
  kind: "shows",
});

/** The signed-in admin, as `fakeMe` names her. */
export const anna: User = {
  id: fakeMe.account.id,
  name: "Anna",
  role: "admin",
  status: "active",
  createdAt: "2026-11-01T10:00:00Z",
  lastActiveAt: "2026-11-06T11:58:00Z",
  libraryAccess: { all: true },
  contentLimit: { preset: "adults", maxAge: null, hideUnrated: false },
  profileCount: 2,
};

/** A user with selected libraries and a custom limit. */
export const jonas: User = {
  id: "2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d6e",
  name: "Jonas",
  role: "user",
  status: "disabled",
  createdAt: "2026-11-02T10:00:00Z",
  libraryAccess: { all: false, libraryIds: [movies.id] },
  contentLimit: { preset: "custom", maxAge: 16, hideUnrated: true },
  profileCount: 1,
};

/** Anna's managed profile with a PIN. */
export const max: Profile = {
  id: "7a6b5c4d-3e2f-4a1b-8c9d-0e1f2a3b4c5d",
  userId: anna.id,
  name: "Max",
  avatarColor: "blue",
  hasPin: true,
  isPrimary: false,
  contentLimit: { preset: "children", maxAge: 6, hideUnrated: true },
  libraryAccess: { all: false, libraryIds: [movies.id, shows.id] },
};

/** The primary profile of Anna, the signed-in admin. */
export const annaPrimary: Profile = {
  id: "6f5e4d3c-2b1a-4f0e-9d8c-7b6a5f4e3d2c",
  userId: anna.id,
  name: "Anna",
  avatarColor: "coral",
  hasPin: false,
  isPrimary: true,
  contentLimit: anna.contentLimit,
  libraryAccess: anna.libraryAccess,
};

/** The primary profile of Jonas; it carries his account access and limit. */
export const jonasPrimary: Profile = {
  id: "5e4d3c2b-1a0f-4e9d-8c7b-6a5f4e3d2c1b",
  userId: jonas.id,
  name: "Jonas",
  avatarColor: "green",
  hasPin: false,
  isPrimary: true,
  contentLimit: jonas.contentLimit,
  libraryAccess: jonas.libraryAccess,
};

/** A signed-in viewer without admin powers. */
export const userMe: Me = {
  account: { id: jonas.id, name: "Jonas", role: "user" },
  currentProfile: {
    id: "5e4d3c2b-1a0f-4e9d-8c7b-6a5f4e3d2c1b",
    name: "Jonas",
    avatarColor: "green",
    hasPin: false,
    isPrimary: true,
    isAdmin: false,
  },
  profiles: [],
};

/** Overrides of {@link createFakeUsersServer}: a fixed answer for one operation. */
export interface FakeUsersOptions {
  readonly update?: () => Response;
  readonly remove?: () => Response;
  readonly list?: () => Response;
  /** Answer of `GET /libraries`; called per request, so a test can fail it once. */
  readonly libraries?: () => Response;
  readonly createProfile?: () => Response;
  readonly updateProfile?: () => Response;
  readonly removeProfile?: () => Response;
}

/** A fake server with mutable accounts, plus what it received. */
export interface FakeUsersServer extends FakeServer {
  readonly users: User[];
  readonly profiles: Profile[];
  readonly bodies: { readonly method: string; readonly path: string; readonly body: unknown }[];
}

function created(users: User[], body: UserCreate): Response {
  const user: User = {
    id: "8f7e6d5c-4b3a-4291-8f7e-6d5c4b3a2918",
    name: body.name,
    role: body.role,
    status: "active",
    createdAt: "2026-11-06T12:00:00Z",
    libraryAccess: body.libraryAccess ?? { all: true },
    contentLimit: body.contentLimit ?? { preset: "adults", maxAge: null, hideUnrated: false },
    profileCount: 1,
  };
  users.push(user);
  return jsonResponse(user, createdStatus);
}

function changed(users: User[], userId: string, body: UserUpdate): Response {
  const index = users.findIndex((user) => user.id === userId);
  const current = users[index];
  if (current === undefined) {
    return problemResponse(404, "not_found");
  }
  const next: User = { ...current, ...body };
  users[index] = next;
  return jsonResponse(next);
}

type FakeUsersState = Pick<FakeUsersServer, "users" | "profiles" | "bodies">;

async function record(state: FakeUsersState, request: Request, path: string): Promise<unknown> {
  const body: unknown = request.method === "DELETE" ? undefined : await request.clone().json();
  state.bodies.push({ method: request.method, path, body });
  return body;
}

function createdProfile(state: FakeUsersState, userId: string, body: ProfileCreate): Response {
  const profile: Profile = {
    id: "4d3c2b1a-0f9e-4d8c-8b7a-6f5e4d3c2b1a",
    userId,
    name: body.name,
    avatarColor: body.avatarColor ?? "coral",
    hasPin: body.pin !== undefined,
    isPrimary: false,
    contentLimit: body.contentLimit ?? { preset: "adults", maxAge: null, hideUnrated: false },
    libraryAccess: body.libraryAccess ?? { all: true },
  };
  state.profiles.push(profile);
  return jsonResponse(profile, createdStatus);
}

function changedProfile(state: FakeUsersState, profileId: string, body: ProfileUpdate): Response {
  const index = state.profiles.findIndex((profile) => profile.id === profileId);
  const current = state.profiles[index];
  if (current === undefined) {
    return problemResponse(404, "not_found");
  }
  const { pin, ...rest } = body;
  const hasPin = pin === undefined ? current.hasPin : pin !== null;
  const next: Profile = { ...current, ...rest, hasPin };
  state.profiles[index] = next;
  return jsonResponse(next);
}

function profileRoutes(state: FakeUsersState, options: FakeUsersOptions): FakeRoute[] {
  return [
    {
      method: "GET",
      path: "/api/v1/users/:userId/profiles",
      respond: ({ params }) =>
        jsonResponse({
          profiles: state.profiles.filter((profile) => profile.userId === params["userId"]),
        }),
    },
    {
      method: "POST",
      path: "/api/v1/users/:userId/profiles",
      respond: async ({ request, url, params }) => {
        const body = (await record(state, request, url.pathname)) as ProfileCreate;
        return options.createProfile?.() ?? createdProfile(state, params["userId"] ?? "", body);
      },
    },
    {
      method: "PATCH",
      path: "/api/v1/profiles/:profileId",
      respond: async ({ request, url, params }) => {
        const body = (await record(state, request, url.pathname)) as ProfileUpdate;
        return options.updateProfile?.() ?? changedProfile(state, params["profileId"] ?? "", body);
      },
    },
    {
      method: "DELETE",
      path: "/api/v1/profiles/:profileId",
      respond: async ({ request, url, params }) => {
        await record(state, request, url.pathname);
        if (options.removeProfile !== undefined) {
          return options.removeProfile();
        }
        const index = state.profiles.findIndex((profile) => profile.id === params["profileId"]);
        state.profiles.splice(index, 1);
        return new Response(null, { status: noContentStatus });
      },
    },
  ];
}

function userRoutes(state: FakeUsersState, options: FakeUsersOptions) {
  const { users } = state;
  const routes: FakeRoute[] = [
    {
      method: "GET",
      path: "/api/v1/users",
      respond: () => options.list?.() ?? jsonResponse({ users }),
    },
    {
      method: "POST",
      path: "/api/v1/users",
      respond: async ({ request, url }) =>
        created(users, (await record(state, request, url.pathname)) as UserCreate),
    },
    {
      method: "PATCH",
      path: "/api/v1/users/:userId",
      respond: async ({ request, url, params }) => {
        const body = (await record(state, request, url.pathname)) as UserUpdate;
        return options.update?.() ?? changed(users, params["userId"] ?? "", body);
      },
    },
    {
      method: "PUT",
      path: "/api/v1/users/:userId/password",
      respond: async ({ request, url }) => {
        await record(state, request, url.pathname);
        return new Response(null, { status: noContentStatus });
      },
    },
    {
      method: "DELETE",
      path: "/api/v1/users/:userId",
      respond: async ({ request, url, params }) => {
        await record(state, request, url.pathname);
        if (options.remove !== undefined) {
          return options.remove();
        }
        users.splice(
          users.findIndex((user) => user.id === params["userId"]),
          1,
        );
        return new Response(null, { status: noContentStatus });
      },
    },
    {
      method: "GET",
      path: "/api/v1/libraries",
      respond: () => options.libraries?.() ?? jsonResponse({ libraries: [movies, shows] }),
    },
  ];
  return routes;
}

/** Creates a fake server that manages Anna (admin, with Max) and Jonas (user). */
export function createFakeUsersServer(
  options: FakeUsersOptions = {},
  extraRoutes: readonly FakeRoute[] = [],
): FakeUsersServer {
  const state: FakeUsersState = {
    users: [anna, jonas],
    profiles: [annaPrimary, max, jonasPrimary],
    bodies: [],
  };
  const server = createFakeServer([
    ...extraRoutes,
    ...profileRoutes(state, options),
    ...userRoutes(state, options),
  ]);
  return { ...server, ...state };
}

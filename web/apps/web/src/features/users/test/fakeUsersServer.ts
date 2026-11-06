import type { Library, Me, Profile, User, UserCreate, UserUpdate } from "@mivo/api-client";

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

const annaPrimary: Profile = {
  id: "6f5e4d3c-2b1a-4f0e-9d8c-7b6a5f4e3d2c",
  userId: anna.id,
  name: "Anna",
  avatarColor: "coral",
  hasPin: false,
  isPrimary: true,
  contentLimit: anna.contentLimit,
  libraryAccess: anna.libraryAccess,
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
}

/** A fake server with mutable accounts, plus what it received. */
export interface FakeUsersServer extends FakeServer {
  readonly users: User[];
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

function userRoutes(server: Pick<FakeUsersServer, "users" | "bodies">, options: FakeUsersOptions) {
  const { users, bodies } = server;
  const record = async (request: Request, path: string): Promise<unknown> => {
    const body: unknown = request.method === "DELETE" ? undefined : await request.clone().json();
    bodies.push({ method: request.method, path, body });
    return body;
  };
  const routes: FakeRoute[] = [
    {
      method: "GET",
      path: "/api/v1/users",
      respond: () => options.list?.() ?? jsonResponse({ users }),
    },
    {
      method: "GET",
      path: "/api/v1/users/:userId/profiles",
      respond: ({ params }) =>
        jsonResponse({ profiles: params["userId"] === anna.id ? [annaPrimary, max] : [] }),
    },
    {
      method: "POST",
      path: "/api/v1/users",
      respond: async ({ request, url }) =>
        created(users, (await record(request, url.pathname)) as UserCreate),
    },
    {
      method: "PATCH",
      path: "/api/v1/users/:userId",
      respond: async ({ request, url, params }) => {
        const body = (await record(request, url.pathname)) as UserUpdate;
        return options.update?.() ?? changed(users, params["userId"] ?? "", body);
      },
    },
    {
      method: "PUT",
      path: "/api/v1/users/:userId/password",
      respond: async ({ request, url }) => {
        await record(request, url.pathname);
        return new Response(null, { status: noContentStatus });
      },
    },
    {
      method: "DELETE",
      path: "/api/v1/users/:userId",
      respond: async ({ request, url, params }) => {
        await record(request, url.pathname);
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
      respond: () => jsonResponse({ libraries: [movies, shows] }),
    },
  ];
  return routes;
}

/** Creates a fake server that manages Anna (admin, with Max) and Jonas (user). */
export function createFakeUsersServer(
  options: FakeUsersOptions = {},
  extraRoutes: readonly FakeRoute[] = [],
): FakeUsersServer {
  const state = { users: [anna, jonas], bodies: [] };
  const server = createFakeServer([...extraRoutes, ...userRoutes(state, options)]);
  return { ...server, ...state };
}

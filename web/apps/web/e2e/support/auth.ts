// Signs the E2E suite in over the API (task M7-28), so the specs start signed in without a
// single UI click. A server without sign-in (no setup endpoint, or the M7-01 stub answering
// 501) gets an empty state and the specs run as before.
import { request, type APIRequestContext, type APIResponse } from "@playwright/test";
import { mkdir, writeFile } from "node:fs/promises";
import { dirname } from "node:path";
import { fileURLToPath } from "node:url";

const setupPath = "/api/v1/setup";
const sessionPath = "/api/v1/auth/session";
const usersPath = "/api/v1/users";

const statusNotFound = 404;
const statusNotImplemented = 501;
// No endpoint, or the contract stub: the server has no sign-in yet.
const signInMissingStatuses: readonly number[] = [statusNotFound, statusNotImplemented];

/** The storage state shape of Playwright's `storageState` option and `storageState()`. */
export type StorageState = Awaited<ReturnType<APIRequestContext["storageState"]>>;

// The api-client exports no auth types before the web app signs in (M7-30), and the user
// administration contract (M7-02) is not in api/openapi.yaml yet; these local shapes follow
// the task contracts and give way to the generated types once both exist.

/** Body of `GET /api/v1/setup`. */
interface SetupState {
  readonly required: boolean;
}

/** Name and password of an account. */
export interface Credentials {
  readonly name: string;
  readonly password: string;
}

/**
 * The administrator the global setup creates on a fresh server. There are no default
 * credentials (ADR 0032), so the suite owns a fixed test account.
 */
export const e2eAdmin: Credentials = { name: "E2E Admin", password: "e2e-admin-password" };

/** Device name of the suite's sessions in the server's device list. */
const e2eDeviceName = "Playwright";

/**
 * File the global setup writes the default storage state to; `playwright.config.ts` passes it
 * to every context. It lies in the output directory, which Playwright empties before the
 * global setup, so a run never reuses the cookie of an earlier server.
 */
export const storageStateFile = fileURLToPath(
  new URL("../../test-results/.auth/storage-state.json", import.meta.url),
);

const emptyState: StorageState = { cookies: [], origins: [] };

/**
 * Creates a request context for `baseUrl` that sends the browser's same-origin headers, so
 * writes pass the ADR 0028 same-origin rule exactly like the web app's.
 */
export function apiContext(baseUrl: string, storageState?: StorageState) {
  return request.newContext({
    baseURL: baseUrl,
    extraHTTPHeaders: { Origin: baseUrl },
    ...(storageState === undefined ? {} : { storageState }),
  });
}

/**
 * Signs in as {@link e2eAdmin} on the server at `baseUrl`, completing the first-run setup when
 * the server still needs it, and returns the resulting storage state.
 * @returns An empty state when the server has no sign-in yet.
 * @throws Error when the server answers the setup state, setup or sign-in unexpectedly.
 */
export async function adminStorageState(baseUrl: string): Promise<StorageState> {
  const context = await apiContext(baseUrl);
  try {
    const setup = await context.get(setupPath);
    if (signInMissingStatuses.includes(setup.status())) {
      return emptyState;
    }
    await assertOk(setup, `GET ${setupPath}`);
    const state = (await setup.json()) as SetupState;
    if (state.required) {
      await post(context, setupPath, signInBody(e2eAdmin));
    } else {
      await post(context, sessionPath, signInBody(e2eAdmin));
    }
    return await context.storageState();
  } finally {
    await context.dispose();
  }
}

/**
 * Signs in as `credentials` on a server whose setup is complete and returns the storage state,
 * e.g. for a second browser context of another account.
 * @throws Error when the sign-in fails.
 */
export async function storageStateFor(
  baseUrl: string,
  credentials: Credentials,
): Promise<StorageState> {
  const context = await apiContext(baseUrl);
  try {
    await post(context, sessionPath, signInBody(credentials));
    return await context.storageState();
  } finally {
    await context.dispose();
  }
}

const adminStates = new Map<string, Promise<StorageState>>();

/**
 * {@link adminStorageState}, signed in once per server and worker. Specs that start their own
 * server override the `storageState` option with it, because the default state belongs to the
 * shared server and browsers send cookies regardless of the port.
 */
export function cachedAdminStorageState(baseUrl: string): Promise<StorageState> {
  let state = adminStates.get(baseUrl);
  if (state === undefined) {
    state = adminStorageState(baseUrl);
    adminStates.set(baseUrl, state);
  }
  return state;
}

/** Signs in on the server at `baseUrl` and writes the state to {@link storageStateFile}. */
export async function writeAdminStorageState(baseUrl: string): Promise<void> {
  const state = await cachedAdminStorageState(baseUrl);
  await mkdir(dirname(storageStateFile), { recursive: true });
  await writeFile(storageStateFile, JSON.stringify(state, null, 2));
}

/** Access of an account or profile to the libraries (M7-02 `LibraryAccess`). */
export interface LibraryAccess {
  readonly all: boolean;
  readonly libraryIds: readonly string[];
}

/** Age limit of a profile (M7-02 `ContentLimit`, ADR 0032 section 7). */
export interface ContentLimit {
  readonly preset: "youngChildren" | "children" | "teens" | "adults" | "custom";
  readonly maxAge: 0 | 6 | 12 | 16 | 18 | null;
  readonly hideUnrated: boolean;
}

/** A further account created by an administrator (`POST /api/v1/users`). */
export interface NewAccount extends Credentials {
  readonly role: "admin" | "user";
  readonly libraryAccess?: LibraryAccess;
  readonly contentLimit?: ContentLimit;
}

/** A managed profile (`POST /api/v1/users/{userId}/profiles`). */
export interface NewProfile {
  readonly name: string;
  readonly avatarColor?: string;
  /** Four digits; omitted, the profile has no PIN. */
  readonly pin?: string;
  readonly contentLimit?: ContentLimit;
  readonly libraryAccess?: LibraryAccess;
}

/** The part of a created account or profile the specs need. */
export interface Created {
  readonly id: string;
}

/**
 * Creates an account through `admin`, a context signed in as an administrator (see
 * {@link apiContext} with {@link adminStorageState}).
 * @throws Error when the server refuses the account.
 */
export async function createAccount(
  admin: APIRequestContext,
  account: NewAccount,
): Promise<Created> {
  return (await post(admin, usersPath, account)) as Created;
}

/**
 * Creates a managed profile of the account `userId` through `context`, signed in as an
 * administrator or as that account.
 * @throws Error when the server refuses the profile.
 */
export async function createProfile(
  context: APIRequestContext,
  userId: string,
  profile: NewProfile,
): Promise<Created> {
  const path = `${usersPath}/${encodeURIComponent(userId)}/profiles`;
  return (await post(context, path, profile)) as Created;
}

function signInBody(credentials: Credentials) {
  return { ...credentials, client: "web", deviceName: e2eDeviceName };
}

async function post(context: APIRequestContext, path: string, body: object): Promise<unknown> {
  const response = await context.post(path, { data: body });
  await assertOk(response, `POST ${path}`);
  const text = await response.text();
  return text === "" ? undefined : (JSON.parse(text) as unknown);
}

async function assertOk(response: APIResponse, what: string): Promise<void> {
  if (!response.ok()) {
    throw new Error(`${what} answered ${String(response.status())}: ${await response.text()}`);
  }
}

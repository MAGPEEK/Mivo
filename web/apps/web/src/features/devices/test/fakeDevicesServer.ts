import type { AccountDevice, Device, Me, SetupState } from "@mivo/api-client";

import {
  createFakeServer,
  fakeMe,
  jsonResponse,
  problemResponse,
  type FakeRoute,
  type FakeServer,
} from "../../../test/fakeApi";

const minute = 60_000;
const day = 24 * 60 * minute;

/** Devices of the fake server, the browser's own one first. */
export function fakeDevices(now = Date.now()): AccountDevice[] {
  return [
    {
      id: "8e7d6c5b-4a3f-4e2d-9c1b-0a9f8e7d6c5b",
      userId: fakeMe.account.id,
      userName: "Anna",
      name: "Firefox on Windows",
      client: "web",
      platform: "Windows",
      lastSeenAt: new Date(now - minute).toISOString(),
      addressClass: "lan",
      current: true,
    },
    {
      id: "2b3c4d5e-6f70-4a81-9b2c-3d4e5f607182",
      userId: "9f8e7d6c-5b4a-4c3d-8e2f-1a0b9c8d7e6f",
      userName: "Jonas",
      name: "Living room TV",
      client: "tv",
      lastSeenAt: new Date(now - 2 * day).toISOString(),
      addressClass: "remote",
      current: false,
    },
  ];
}

/** A fake server with devices whose revokes end sessions, plus its device list. */
export interface FakeDevicesServer extends FakeServer {
  readonly devices: AccountDevice[];
}

function withoutAccount(device: AccountDevice): Device {
  return {
    id: device.id,
    name: device.name,
    client: device.client,
    ...(device.platform === undefined ? {} : { platform: device.platform }),
    lastSeenAt: device.lastSeenAt,
    addressClass: device.addressClass,
    current: device.current,
  };
}

/** Answers both device lists and revokes; revoking the current device signs the browser out. */
export function createFakeDevicesServer(routes: readonly FakeRoute[] = []): FakeDevicesServer {
  const devices = fakeDevices();
  const state = { signedIn: true };
  const revoke = (deviceId: string | undefined): Response => {
    const index = devices.findIndex((device) => device.id === deviceId);
    if (index < 0) {
      return problemResponse(404, "not_found");
    }
    const [removed] = devices.splice(index, 1);
    state.signedIn = removed?.current !== true;
    return new Response(null, { status: 204 });
  };
  const server = createFakeServer([
    ...routes,
    {
      method: "GET",
      path: "/api/v1/setup",
      respond: () => jsonResponse({ required: false } satisfies SetupState),
    },
    {
      method: "GET",
      path: "/api/v1/me",
      respond: () =>
        state.signedIn
          ? jsonResponse(fakeMe satisfies Me)
          : problemResponse(401, "unauthenticated"),
    },
    {
      method: "DELETE",
      path: "/api/v1/auth/session",
      respond: () =>
        state.signedIn
          ? new Response(null, { status: 204 })
          : problemResponse(401, "unauthenticated"),
    },
    {
      method: "GET",
      path: "/api/v1/me/devices",
      respond: () => jsonResponse({ devices: devices.map(withoutAccount) }),
    },
    { method: "GET", path: "/api/v1/devices", respond: () => jsonResponse({ devices }) },
    {
      method: "DELETE",
      path: "/api/v1/me/devices/:deviceId",
      respond: ({ params }) => revoke(params["deviceId"]),
    },
    {
      method: "DELETE",
      path: "/api/v1/devices/:deviceId",
      respond: ({ params }) => revoke(params["deviceId"]),
    },
  ]);
  return { ...server, devices };
}

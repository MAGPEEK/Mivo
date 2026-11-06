import {
  ApiError,
  noResponseStatus,
  requestData,
  type AccountDeviceList,
  type ApiClient,
  type DeviceList,
} from "@mivo/api-client";
import { queryOptions } from "@tanstack/react-query";

/** Prefix of every device cache key. */
export const devicesQueryKey = ["devices"] as const;

/** Cache key of `GET /api/v1/me/devices`. */
export const ownDevicesQueryKey = [...devicesQueryKey, "own"] as const;

/** Cache key of `GET /api/v1/devices`. */
export const allDevicesQueryKey = [...devicesQueryKey, "all"] as const;

const maxNetworkRetries = 1;

// A 403 or 501 does not change on a retry; only a lost connection is tried once more.
function retryOnlyNetwork(failureCount: number, error: unknown): boolean {
  const answered = error instanceof ApiError && error.status !== noResponseStatus;
  return !answered && failureCount < maxNetworkRetries;
}

/** Query options for the devices signed in to the own account. */
export function ownDevicesQueryOptions(client: ApiClient) {
  return queryOptions<DeviceList>({
    queryKey: ownDevicesQueryKey,
    queryFn: ({ signal }) => requestData(client.GET("/api/v1/me/devices", { signal })),
    retry: retryOnlyNetwork,
  });
}

/** Query options for the devices of every account (administration). */
export function allDevicesQueryOptions(client: ApiClient) {
  return queryOptions<AccountDeviceList>({
    queryKey: allDevicesQueryKey,
    queryFn: ({ signal }) => requestData(client.GET("/api/v1/devices", { signal })),
    retry: retryOnlyNetwork,
  });
}

/** Signs a device of the own account out. Resolves on 204. */
export async function revokeOwnDevice(client: ApiClient, deviceId: string): Promise<void> {
  await requestData(
    client.DELETE("/api/v1/me/devices/{deviceId}", { params: { path: { deviceId } } }),
  );
}

/** Signs a device of any account out (administration). Resolves on 204. */
export async function revokeAnyDevice(client: ApiClient, deviceId: string): Promise<void> {
  await requestData(
    client.DELETE("/api/v1/devices/{deviceId}", { params: { path: { deviceId } } }),
  );
}

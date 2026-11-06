import { useQuery } from "@tanstack/react-query";

import { useApiClient } from "../../../shared/api";
import { allDevicesQueryOptions, ownDevicesQueryOptions } from "../api/devicesApi";
import { devicesFailure, type DeviceRow, type DevicesFailure } from "../lib/devices";

/** Which devices a page lists: the own account's or every account's (administration). */
export type DeviceScope = "own" | "all";

/** A device list as a page shows it. */
export interface DeviceListState {
  /** The devices, most recently seen first; undefined until loaded or after a failure. */
  readonly devices: readonly DeviceRow[] | undefined;
  readonly loading: boolean;
  /** Why the list could not be loaded; undefined otherwise. */
  readonly failure: DevicesFailure | undefined;
  readonly retry: () => void;
}

/** Loads the devices of `scope`; a server without accounts (501) reports `notAvailable`. */
export function useDeviceList(scope: DeviceScope): DeviceListState {
  const client = useApiClient();
  // Both queries are declared so the hook order stays fixed; only the scope's one runs.
  const own = useQuery({ ...ownDevicesQueryOptions(client), enabled: scope === "own" });
  const all = useQuery({ ...allDevicesQueryOptions(client), enabled: scope === "all" });
  const query = scope === "own" ? own : all;
  return {
    devices: query.data?.devices,
    loading: query.isPending,
    failure: query.isError ? devicesFailure(query.error) : undefined,
    retry: () => {
      void query.refetch();
    },
  };
}

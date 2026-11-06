import { useState } from "react";

import { filterByClient, isClientFilter, type ClientFilter, type DeviceRow } from "../lib/devices";

/** The client-type filter of the admin device list. */
export interface ClientFilterState {
  readonly filter: ClientFilter;
  /** Takes the value of the filter select; unknown values are ignored. */
  readonly select: (value: string) => void;
  /** The devices that pass the filter. */
  readonly apply: (devices: readonly DeviceRow[]) => readonly DeviceRow[];
}

/** Holds the client-type filter; it starts with every client. */
export function useClientFilter(): ClientFilterState {
  const [filter, setFilter] = useState<ClientFilter>("all");
  return {
    filter,
    select: (value) => {
      if (isClientFilter(value)) {
        setFilter(value);
      }
    },
    apply: (devices) => filterByClient(devices, filter),
  };
}

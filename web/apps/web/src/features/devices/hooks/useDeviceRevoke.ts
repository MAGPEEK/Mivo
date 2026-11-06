import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { useSignOut } from "../../auth";
import { useApiClient } from "../../../shared/api";
import { devicesQueryKey, revokeAnyDevice, revokeOwnDevice } from "../api/devicesApi";
import { devicesFailure, type DeviceRow, type DevicesFailure } from "../lib/devices";
import type { DeviceScope } from "./useDeviceList";

/** State and actions of the "Revoke device" confirmation. */
export interface DeviceRevoke {
  /** The device the dialog asks about; undefined while the dialog is closed. */
  readonly target: DeviceRow | undefined;
  readonly pending: boolean;
  /** Why the last revoke failed, for display inside the dialog; undefined otherwise. */
  readonly failure: DevicesFailure | undefined;
  /** Opens the confirmation for `device`. */
  readonly ask: (device: DeviceRow) => void;
  readonly cancel: () => void;
  readonly confirm: () => void;
}

/**
 * Drives revoking a device: confirmation first, then the scope's DELETE. Revoking this
 * browser's own device ended its session, so it finishes like "Sign out" and opens the
 * sign-in page; any other device just leaves the lists, which reload.
 */
export function useDeviceRevoke(scope: DeviceScope): DeviceRevoke {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const signOut = useSignOut();
  const [target, setTarget] = useState<DeviceRow | undefined>(undefined);
  const mutation = useMutation<unknown, Error, DeviceRow>({
    mutationFn: (device) =>
      scope === "own" ? revokeOwnDevice(client, device.id) : revokeAnyDevice(client, device.id),
    onSuccess: async (_result, device) => {
      if (device.current) {
        // The session is gone already; signing out clears the cache and shows sign-in.
        signOut.signOut();
        return;
      }
      setTarget(undefined);
      await queryClient.invalidateQueries({ queryKey: devicesQueryKey });
    },
  });
  const { reset, mutate } = mutation;
  return {
    target,
    pending: mutation.isPending || signOut.pending,
    failure: mutation.isError ? devicesFailure(mutation.error) : undefined,
    ask: (device) => {
      reset();
      setTarget(device);
    },
    cancel: () => {
      // Closing mid-request would hide the outcome of a revoke that still runs.
      if (!mutation.isPending) {
        reset();
        setTarget(undefined);
      }
    },
    confirm: () => {
      if (target !== undefined && !mutation.isPending) {
        mutate(target);
      }
    },
  };
}

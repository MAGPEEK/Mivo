import type { Me, ProfileSummary } from "@mivo/api-client";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { useSignOut } from "../../auth";
import { ownDevicesPath } from "../../devices";
import { canChangePassword } from "../lib/profileModel";
import { useSwitchProfile } from "./useSwitchProfile";

/** One action of the profile menu. */
export interface ProfileMenuEntry {
  readonly id: "switch" | "changePassword" | "devices" | "signOut";
  readonly label: string;
  readonly onSelect: () => void;
}

/** State and actions of the profile menu (SH-07). */
export interface ProfileMenuState {
  readonly entries: readonly ProfileMenuEntry[];
  readonly passwordDialogOpen: boolean;
  readonly closePasswordDialog: () => void;
  /** True when switching profile or signing out failed; the device is unchanged. */
  readonly failed: boolean;
}

/**
 * The entries of the profile menu: "Switch profile", "Change password…" for the primary profile
 * (the account password belongs to it), "Devices" (the account's own sessions) and "Sign out".
 */
export function useProfileMenu(me: Me, profile: ProfileSummary): ProfileMenuState {
  const { t } = useTranslation("profiles");
  const [passwordDialogOpen, setPasswordDialogOpen] = useState(false);
  const switching = useSwitchProfile(me);
  const signOut = useSignOut();
  const navigate = useNavigate();
  const entries: ProfileMenuEntry[] = [
    { id: "switch", label: t("menu.switch"), onSelect: switching.switchProfile },
  ];
  if (canChangePassword(profile)) {
    entries.push({
      id: "changePassword",
      label: t("menu.changePassword"),
      onSelect: () => {
        setPasswordDialogOpen(true);
      },
    });
  }
  entries.push(
    {
      id: "devices",
      label: t("menu.devices"),
      onSelect: () => {
        void navigate({ to: ownDevicesPath });
      },
    },
    {
      id: "signOut",
      label: signOut.pending ? t("menu.signOutPending") : t("menu.signOut"),
      onSelect: signOut.signOut,
    },
  );
  return {
    entries,
    passwordDialogOpen,
    closePasswordDialog: () => {
      setPasswordDialogOpen(false);
    },
    failed: switching.failed || signOut.failed,
  };
}

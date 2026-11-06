import type { Me, ProfileSummary } from "@mivo/api-client";
import { Fragment, useId, useRef, type KeyboardEvent, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { useMenu } from "../../../shared/ui";
import { ChangePasswordDialog, useSessionSnapshot } from "../../auth";
import { useProfileMenu, type ProfileMenuEntry } from "../hooks/useProfileMenu";
import { suggestsOwnPin } from "../lib/profileModel";
import { Avatar } from "./Avatar";

const popupClass =
  "absolute top-full right-0 z-40 mt-2 w-72 rounded-lg border border-divider bg-surface p-1.5 shadow-overlay";
const failureClass =
  "absolute top-full right-0 mt-2 w-64 rounded-md bg-surface-raised p-2 text-sm text-accent";
const itemClass =
  "flex w-full items-center rounded-sm px-2.5 py-2 text-left text-sm text-text hover:bg-divider";

function ProfileHeader({ me, profile }: { readonly me: Me; readonly profile: ProfileSummary }) {
  const { t } = useTranslation("profiles");
  return (
    <div className="border-b border-divider px-2 pt-2 pb-2.5">
      <div className="flex items-center gap-2.5">
        <Avatar name={profile.name} color={profile.avatarColor} size="medium" />
        <div className="min-w-0">
          <p className="truncate font-semibold text-text">{profile.name}</p>
          <p className="flex flex-wrap items-center gap-1.5 text-xs text-text-muted">
            {profile.isAdmin && (
              <span className="rounded-sm border border-accent px-1.5 font-medium text-accent">
                {t("menu.admin")}
              </span>
            )}
            {profile.isPrimary ? t("menu.primary") : t("menu.managed")}
          </p>
        </div>
      </div>
      {suggestsOwnPin(me) && <p className="mt-2 text-xs text-text-muted">{t("menu.pinHint")}</p>}
    </div>
  );
}

interface ProfileMenuViewProps {
  readonly me: Me;
  readonly profile: ProfileSummary;
}

interface MenuItemsProps {
  readonly id: string;
  readonly entries: readonly ProfileMenuEntry[];
  readonly itemRefs: RefObject<(HTMLButtonElement | null)[]>;
  readonly onKeyDown: (event: KeyboardEvent) => void;
  readonly onChoose: (entry: ProfileMenuEntry) => void;
}

function MenuItems({ id, entries, itemRefs, onKeyDown, onChoose }: MenuItemsProps) {
  const { t } = useTranslation("profiles");
  return (
    <div
      id={id}
      role="menu"
      aria-label={t("menu.label")}
      tabIndex={-1}
      onKeyDown={onKeyDown}
      className="pt-1"
    >
      {entries.map((entry, index) => (
        <Fragment key={entry.id}>
          {entry.id === "signOut" && (
            <div role="separator" className="my-1 border-t border-divider" />
          )}
          <button
            ref={(element) => {
              itemRefs.current[index] = element;
            }}
            type="button"
            role="menuitem"
            tabIndex={-1}
            onClick={() => {
              onChoose(entry);
            }}
            className={itemClass}
          >
            {entry.label}
          </button>
        </Fragment>
      ))}
    </div>
  );
}

// Only rendered on a failure: an always-present alert would sit on every page.
function MenuFailure() {
  const { t } = useTranslation("profiles");
  return (
    <p role="alert" className={failureClass}>
      {t("menu.failed")}
    </p>
  );
}

function ProfileMenuView({ me, profile }: ProfileMenuViewProps) {
  const { t } = useTranslation("profiles");
  const state = useProfileMenu(me, profile);
  const menuId = useId();
  const triggerRef = useRef<HTMLButtonElement>(null);
  const popupRef = useRef<HTMLDivElement>(null);
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const menu = useMenu(
    {
      labels: state.entries.map((entry) => entry.label),
      disabled: state.entries.map(() => false),
    },
    { trigger: triggerRef, menu: popupRef, items: itemRefs },
  );
  return (
    <div className="relative">
      <button
        ref={triggerRef}
        type="button"
        aria-haspopup="menu"
        aria-expanded={menu.open}
        aria-controls={menu.open ? menuId : undefined}
        aria-label={t("menu.trigger", { name: profile.name })}
        onClick={menu.toggle}
        onKeyDown={menu.onTriggerKeyDown}
        className="grid rounded-full"
      >
        <Avatar name={profile.name} color={profile.avatarColor} size="small" />
      </button>
      {menu.open && (
        <div ref={popupRef} className={popupClass}>
          <ProfileHeader me={me} profile={profile} />
          <MenuItems
            id={menuId}
            entries={state.entries}
            itemRefs={itemRefs}
            onKeyDown={menu.onMenuKeyDown}
            onChoose={(entry) => {
              menu.close(true);
              entry.onSelect();
            }}
          />
        </div>
      )}
      {state.failed && !menu.open && <MenuFailure />}
      {state.passwordDialogOpen && <ChangePasswordDialog onClose={state.closePasswordDialog} />}
    </div>
  );
}

/**
 * The profile menu in the top bar (SH-07): avatar button, the current profile with its kind
 * and admin marker, "Switch profile", "Change password…" and "Sign out". Without accounts
 * (a server answering `501`) it renders nothing, so the app looks as before.
 */
export function ProfileMenu() {
  const { authAvailable, me } = useSessionSnapshot();
  const profile = me?.currentProfile ?? null;
  if (!authAvailable || me === undefined || profile === null) {
    return null;
  }
  return <ProfileMenuView me={me} profile={profile} />;
}

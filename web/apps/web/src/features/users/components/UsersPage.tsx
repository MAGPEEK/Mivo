import type { Profile, User } from "@mivo/api-client";
import { productName } from "@mivo/ui-tokens";
import { useRef, useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import {
  ConfirmDialog,
  EmptyState,
  InfoIcon,
  Skeleton,
  primaryButtonClass,
  secondaryButtonClass,
} from "../../../shared/ui";
import { SettingsBreadcrumb, SettingsNav } from "../../settings";
import { useUserList, type UserListState } from "../hooks/useUserList";
import { useUserRemoval, type UserRemoval } from "../hooks/useUserRemoval";
import { usersNamespace } from "../locales";
import { ProfileDialog } from "../profiles/components/ProfileDialog";
import { RemoveProfileDialog } from "../profiles/components/RemoveProfileDialog";
import { useProfileRemoval } from "../profiles/hooks/useProfileRemoval";
import { UserDialog } from "./UserDialog";
import { UsersTable, type UsersTableProps } from "./UsersTable";

/**
 * Which dialog is open: none, "Add user", the editor of one account, or the editor of a
 * profile of `user` ("Create managed profile" without `profile`).
 */
type DialogTarget =
  | null
  | { readonly mode: "create" }
  | { readonly mode: "edit"; readonly user: User }
  | { readonly mode: "profile"; readonly user: User; readonly profile?: Profile };

type UsersContentProps = Omit<UsersTableProps, "entries" | "now"> & {
  readonly list: UserListState;
};

function UsersContent({ list, ...actions }: UsersContentProps) {
  const { t } = useTranslation(usersNamespace);
  const [now] = useState(() => new Date());
  switch (list.status) {
    case "loading":
      return (
        <div role="status" className="w-60">
          <Skeleton label={t("users.loading")} className="h-5 w-full" />
        </div>
      );
    case "unavailable":
      return (
        <EmptyState
          icon={<InfoIcon className="size-10" />}
          title={t("users.unavailable.title")}
          description={t("users.unavailable.hint", { productName })}
        />
      );
    case "forbidden":
      return <p role="alert">{t("users.forbidden")}</p>;
    case "error":
      return (
        <div role="alert" className="flex flex-col items-start gap-3">
          <p>{t("users.loadFailed")}</p>
          <button type="button" onClick={list.retry} className={secondaryButtonClass}>
            {t("users.retry")}
          </button>
        </div>
      );
    case "ready":
      return <UsersTable entries={list.entries} now={now} {...actions} />;
  }
}

function RemoveDialog({ removal }: { readonly removal: UserRemoval }) {
  const { t } = useTranslation(usersNamespace);
  if (removal.target === null) {
    return null;
  }
  return (
    <ConfirmDialog
      title={t("users.remove.title")}
      description={t("users.remove.description", { name: removal.target.name })}
      confirmLabel={t("users.remove.confirm")}
      cancelLabel={t("users.remove.cancel")}
      pending={removal.pending}
      error={removal.errorText ?? undefined}
      onConfirm={removal.confirm}
      onCancel={removal.cancel}
    />
  );
}

interface PageHeaderProps {
  readonly headingRef: RefObject<HTMLHeadingElement | null>;
  /** Present while accounts can be added. */
  readonly onAdd: (() => void) | undefined;
}

function PageHeader({ headingRef, onAdd }: PageHeaderProps) {
  const { t } = useTranslation(usersNamespace);
  return (
    <>
      <SettingsBreadcrumb title={t("users.heading")} />
      <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 ref={headingRef} tabIndex={-1} className="text-3xl font-semibold">
            {t("users.heading")}
          </h1>
          <p className="mt-1 text-text-muted">{t("users.description")}</p>
        </div>
        {onAdd !== undefined && (
          <button type="button" onClick={onAdd} className={primaryButtonClass}>
            {t("users.add")}
          </button>
        )}
      </div>
      <SettingsNav />
    </>
  );
}

interface OpenDialogProps {
  readonly dialog: DialogTarget;
  readonly onClose: () => void;
  readonly onSaved: (announcement: string) => void;
}

// The account or profile editor that is open; remounted per target, so each starts fresh.
function OpenDialog({ dialog, onClose, onSaved }: OpenDialogProps) {
  const { t } = useTranslation(usersNamespace);
  if (dialog === null) {
    return null;
  }
  if (dialog.mode === "profile") {
    return (
      <ProfileDialog
        key={dialog.profile?.id ?? `new-${dialog.user.id}`}
        owner={dialog.user}
        profile={dialog.profile}
        namesOwner
        onClose={onClose}
        onSaved={(name) => {
          onSaved(t("profiles.announce.saved", { name }));
        }}
      />
    );
  }
  return (
    <UserDialog
      key={dialog.mode === "edit" ? dialog.user.id : "create"}
      user={dialog.mode === "edit" ? dialog.user : undefined}
      onClose={onClose}
      onSaved={(name) => {
        onSaved(t("users.announce.saved", { name }));
      }}
    />
  );
}

// Deleting an account or a profile: the deleted row takes the focused button with it, so
// `onRemoved` gets the text to announce and moves the focus to a stable landmark.
function useRemovals(onRemoved: (announcement: string) => void) {
  const { t } = useTranslation(usersNamespace);
  const account = useUserRemoval(() => {
    onRemoved(t("users.announce.removed"));
  });
  const profile = useProfileRemoval(() => {
    onRemoved(t("profiles.announce.removed"));
  });
  return { account, profile };
}

/** Admin › Users: every account with its managed profiles, and the dialogs to manage them. */
export function UsersPage() {
  const list = useUserList();
  const headingRef = useRef<HTMLHeadingElement>(null);
  const [dialog, setDialog] = useState<DialogTarget>(null);
  const [announcement, setAnnouncement] = useState("");
  const removals = useRemovals((text) => {
    setAnnouncement(text);
    headingRef.current?.focus();
  });
  const close = () => {
    setDialog(null);
  };
  const addUser = () => {
    setDialog({ mode: "create" });
  };
  return (
    <>
      <PageHeader headingRef={headingRef} onAdd={list.status === "ready" ? addUser : undefined} />
      <p role="status" className="sr-only">
        {announcement}
      </p>
      <UsersContent
        list={list}
        onEdit={(user) => {
          setDialog({ mode: "edit", user });
        }}
        onDelete={removals.account.ask}
        onAddProfile={(user) => {
          setDialog({ mode: "profile", user });
        }}
        onEditProfile={(user, profile) => {
          setDialog({ mode: "profile", user, profile });
        }}
        onDeleteProfile={removals.profile.ask}
      />
      <OpenDialog
        dialog={dialog}
        onClose={close}
        onSaved={(text) => {
          setAnnouncement(text);
          close();
        }}
      />
      <RemoveDialog removal={removals.account} />
      <RemoveProfileDialog removal={removals.profile} />
    </>
  );
}

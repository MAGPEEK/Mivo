import type { User } from "@mivo/api-client";
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
import { UserDialog } from "./UserDialog";
import { UsersTable } from "./UsersTable";

/** Which dialog is open: none, "Add user", or the editor of one account. */
type DialogTarget =
  null | { readonly mode: "create" } | { readonly mode: "edit"; readonly user: User };

interface UsersContentProps {
  readonly list: UserListState;
  readonly onEdit: (user: User) => void;
  readonly onDelete: (user: User) => void;
}

function UsersContent({ list, onEdit, onDelete }: UsersContentProps) {
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
      return <UsersTable entries={list.entries} now={now} onEdit={onEdit} onDelete={onDelete} />;
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

/** Admin › Users: every account with its managed profiles, and the dialogs to manage them. */
export function UsersPage() {
  const { t } = useTranslation(usersNamespace);
  const list = useUserList();
  const headingRef = useRef<HTMLHeadingElement>(null);
  const [dialog, setDialog] = useState<DialogTarget>(null);
  const [announcement, setAnnouncement] = useState("");
  // The deleted row takes the focused button with it; the heading is the stable landmark left.
  const removal = useUserRemoval(() => {
    setAnnouncement(t("users.announce.removed"));
    headingRef.current?.focus();
  });
  const close = () => {
    setDialog(null);
  };
  return (
    <>
      <PageHeader
        headingRef={headingRef}
        onAdd={
          list.status === "ready"
            ? () => {
                setDialog({ mode: "create" });
              }
            : undefined
        }
      />
      <p role="status" className="sr-only">
        {announcement}
      </p>
      <UsersContent
        list={list}
        onEdit={(user) => {
          setDialog({ mode: "edit", user });
        }}
        onDelete={removal.ask}
      />
      {dialog !== null && (
        <UserDialog
          key={dialog.mode === "edit" ? dialog.user.id : "create"}
          user={dialog.mode === "edit" ? dialog.user : undefined}
          onClose={close}
          onSaved={(name) => {
            setAnnouncement(t("users.announce.saved", { name }));
            close();
          }}
        />
      )}
      <RemoveDialog removal={removal} />
    </>
  );
}

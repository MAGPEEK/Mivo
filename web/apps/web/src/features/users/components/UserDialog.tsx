import type { AccountRole, User } from "@mivo/api-client";
import { useId, type SyntheticEvent } from "react";
import { useTranslation } from "react-i18next";

import { ModalDialog, primaryButtonClass, secondaryButtonClass } from "../../../shared/ui";
import { useUserDialog, type UserDialogState } from "../hooks/useUserDialog";
import { maxNameLength, maxPasswordBytes, minPasswordBytes } from "../lib/userDraft";
import { usersNamespace } from "../locales";
import { ContentLimitField } from "./ContentLimitField";
import { LibraryAccessField } from "./LibraryAccessField";
import { TextInput, inputClass } from "./TextInput";

/** Props of {@link UserDialog}. */
export interface UserDialogProps {
  /** The account to edit; omitted for "Add user". */
  readonly user?: User | undefined;
  readonly onClose: () => void;
  /** Runs after a successful save with the saved name. */
  readonly onSaved: (name: string) => void;
}

const roles = ["user", "admin"] as const satisfies readonly AccountRole[];

function PasswordSection({ dialog }: { readonly dialog: UserDialogState }) {
  const { t } = useTranslation(usersNamespace);
  const limits = { min: minPasswordBytes, max: maxPasswordBytes };
  if (dialog.creating) {
    return (
      <TextInput
        type="password"
        label={t("users.dialog.password")}
        value={dialog.draft.password}
        onChange={dialog.setPassword}
        autoComplete="new-password"
        hint={t("users.dialog.passwordHint", limits)}
        error={dialog.errors.password}
      />
    );
  }
  if (!dialog.resettingPassword) {
    return (
      <div className="mb-4">
        <button type="button" onClick={dialog.startPasswordReset} className={secondaryButtonClass}>
          {t("users.dialog.resetPassword")}
        </button>
      </div>
    );
  }
  return (
    <div className="mb-4 flex flex-col items-start">
      <div className="w-full">
        <TextInput
          type="password"
          label={t("users.dialog.newPassword")}
          value={dialog.draft.password}
          onChange={dialog.setPassword}
          autoComplete="new-password"
          hint={t("users.dialog.newPasswordHint")}
          error={dialog.errors.password}
          focusOnMount
        />
      </div>
      <button type="button" onClick={dialog.cancelPasswordReset} className={secondaryButtonClass}>
        {t("users.dialog.keepPassword")}
      </button>
    </div>
  );
}

function RoleField({ dialog }: { readonly dialog: UserDialogState }) {
  const { t } = useTranslation(usersNamespace);
  const id = useId();
  return (
    <div className="mb-4 flex flex-col gap-1">
      <label htmlFor={id} className="text-sm font-medium text-text">
        {t("users.dialog.role")}
      </label>
      <select
        id={id}
        value={dialog.draft.role}
        onChange={(event) => {
          const role = roles.find((known) => known === event.target.value);
          if (role !== undefined) {
            dialog.setRole(role);
          }
        }}
        className={inputClass}
      >
        {roles.map((role) => (
          <option key={role} value={role}>
            {t(`users.role.${role}`)}
          </option>
        ))}
      </select>
    </div>
  );
}

function DisabledSwitch({ dialog }: { readonly dialog: UserDialogState }) {
  const { t } = useTranslation(usersNamespace);
  const hintId = useId();
  return (
    <div className="mb-4">
      <label className="flex items-center gap-2 text-sm font-medium text-text">
        <input
          type="checkbox"
          role="switch"
          aria-describedby={hintId}
          checked={dialog.draft.disabled}
          onChange={(event) => {
            dialog.setDisabled(event.target.checked);
          }}
        />
        {t("users.dialog.disabled")}
      </label>
      <p id={hintId} className="mt-1 text-xs text-text-muted">
        {t("users.dialog.disabledHint")}
      </p>
    </div>
  );
}

function AccountFields({ dialog }: { readonly dialog: UserDialogState }) {
  const { t } = useTranslation(usersNamespace);
  return (
    <div>
      <TextInput
        label={t("users.dialog.name")}
        value={dialog.draft.name}
        onChange={dialog.setName}
        autoComplete="off"
        maxLength={maxNameLength}
        error={dialog.errors.name}
        initialFocus
      />
      <PasswordSection dialog={dialog} />
      <RoleField dialog={dialog} />
      {!dialog.creating && <DisabledSwitch dialog={dialog} />}
    </div>
  );
}

function AccessFields({ dialog }: { readonly dialog: UserDialogState }) {
  return (
    <div>
      <LibraryAccessField
        access={dialog.draft.access}
        libraries={dialog.libraries}
        locked={dialog.draft.role === "admin"}
        error={dialog.errors.libraryAccess}
        onAllChange={dialog.setAllLibraries}
        onLibraryChange={dialog.setLibrary}
      />
      <ContentLimitField
        limit={dialog.draft.limit}
        error={dialog.errors.contentLimit}
        onPresetChange={dialog.setPreset}
        onCustomAgeChange={dialog.setCustomAge}
        onHideUnratedChange={dialog.setHideUnrated}
      />
    </div>
  );
}

/** "Add user" or "Edit <name>": account, password, role, access and content limit (US-20…22). */
export function UserDialog({ user, onClose, onSaved }: UserDialogProps) {
  const { t } = useTranslation(usersNamespace);
  const formId = useId();
  const dialog = useUserDialog(user, onSaved);
  const title =
    user === undefined
      ? t("users.dialog.createTitle")
      : t("users.dialog.editTitle", { name: user.name });
  const submit = (event: SyntheticEvent) => {
    event.preventDefault();
    dialog.submit();
  };
  return (
    <ModalDialog
      title={title}
      closeLabel={t("users.dialog.close")}
      onClose={onClose}
      fullScreenOnPhone
      footer={
        <>
          <p role="alert" className="text-sm text-accent empty:hidden">
            {dialog.errors.form}
          </p>
          <div className="ml-auto flex gap-3">
            <button type="button" onClick={onClose} className={secondaryButtonClass}>
              {t("users.dialog.cancel")}
            </button>
            <button
              type="submit"
              form={formId}
              aria-disabled={dialog.pending}
              className={primaryButtonClass}
            >
              {dialog.pending ? t("users.dialog.saving") : t("users.dialog.save")}
            </button>
          </div>
        </>
      }
    >
      <form id={formId} noValidate onSubmit={submit} className="grid gap-x-6 sm:grid-cols-2">
        <AccountFields dialog={dialog} />
        <AccessFields dialog={dialog} />
      </form>
    </ModalDialog>
  );
}

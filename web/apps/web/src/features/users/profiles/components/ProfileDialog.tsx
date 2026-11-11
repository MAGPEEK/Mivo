import type { Profile } from "@mivo/api-client";
import { useId, type SyntheticEvent } from "react";
import { useTranslation } from "react-i18next";

import { ModalDialog, primaryButtonClass, secondaryButtonClass } from "../../../../shared/ui";
import { ContentLimitField } from "../../components/ContentLimitField";
import { TextInput } from "../../components/TextInput";
import { maxNameLength } from "../../lib/userDraft";
import { usersNamespace } from "../../locales";
import { useProfileDialog, type ProfileDialogState } from "../hooks/useProfileDialog";
import type { ProfileOwner } from "../lib/profileDraft";
import { AvatarColorField } from "./AvatarColorField";
import { PinField } from "./PinField";
import { ProfileLibrariesField } from "./ProfileLibrariesField";

/** Props of {@link ProfileDialog}. */
export interface ProfileDialogProps {
  /** The account the profile belongs to. */
  readonly owner: ProfileOwner;
  /** The profile to edit; omitted for "Create managed profile". */
  readonly profile?: Profile | undefined;
  /** Names the account in the title; for Admin › Users, where several accounts are listed. */
  readonly namesOwner?: boolean;
  readonly onClose: () => void;
  /** Runs after a successful save with the saved name. */
  readonly onSaved: (name: string) => void;
}

function IdentityFields({ dialog }: { readonly dialog: ProfileDialogState }) {
  const { t } = useTranslation(usersNamespace);
  return (
    <div>
      {dialog.primary ? (
        <p className="mb-4 text-sm text-text-muted">{t("profiles.dialog.primaryHint")}</p>
      ) : (
        <TextInput
          label={t("users.dialog.name")}
          value={dialog.draft.name}
          onChange={dialog.setName}
          autoComplete="off"
          maxLength={maxNameLength}
          error={dialog.errors.name}
          initialFocus
        />
      )}
      <AvatarColorField color={dialog.draft.avatarColor} onChange={dialog.setAvatarColor} />
      <PinField
        pin={dialog.draft.pin}
        hasPin={dialog.hasPin}
        error={dialog.errors.pin}
        onType={dialog.typePin}
        onAction={dialog.setPinAction}
      />
    </div>
  );
}

function AccessFields({ dialog }: { readonly dialog: ProfileDialogState }) {
  const { t } = useTranslation(usersNamespace);
  return (
    <div>
      <ProfileLibrariesField
        access={dialog.draft.access}
        choices={dialog.choices}
        status={dialog.librariesStatus}
        onRetry={dialog.retryLibraries}
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
      <p className="mb-4 text-xs text-text-muted">{t("profiles.dialog.limitHint")}</p>
    </div>
  );
}

function DialogFooter(props: {
  readonly dialog: ProfileDialogState;
  readonly formId: string;
  readonly onClose: () => void;
}) {
  const { t } = useTranslation(usersNamespace);
  const { dialog } = props;
  return (
    <>
      <p role="alert" className="text-sm text-accent empty:hidden">
        {dialog.errors.form}
      </p>
      <div className="ml-auto flex gap-3">
        <button type="button" onClick={props.onClose} className={secondaryButtonClass}>
          {t("users.dialog.cancel")}
        </button>
        <button
          type="submit"
          form={props.formId}
          aria-disabled={!dialog.canSave}
          className={primaryButtonClass}
        >
          {dialog.pending ? t("users.dialog.saving") : t("users.dialog.save")}
        </button>
      </div>
    </>
  );
}

function useDialogTitle({ owner, profile, namesOwner }: ProfileDialogProps): string {
  const { t } = useTranslation(usersNamespace);
  if (profile !== undefined) {
    return t("profiles.dialog.editTitle", { name: profile.name });
  }
  return namesOwner === true
    ? t("profiles.dialog.createTitleFor", { name: owner.name })
    : t("profiles.dialog.createTitle");
}

/**
 * "Create managed profile" or "Edit profile <name>" (US-10): name, avatar colour, PIN,
 * libraries and content limit. A primary profile shows only its colour and PIN; the rest
 * belongs to its account.
 */
export function ProfileDialog(props: ProfileDialogProps) {
  const { owner, profile, onClose, onSaved } = props;
  const { t } = useTranslation(usersNamespace);
  const formId = useId();
  const title = useDialogTitle(props);
  const dialog = useProfileDialog({ owner, profile }, onSaved);
  const submit = (event: SyntheticEvent) => {
    event.preventDefault();
    dialog.submit();
  };
  return (
    <ModalDialog
      title={title}
      closeLabel={t("users.dialog.close")}
      onClose={onClose}
      size={dialog.primary ? "narrow" : "wide"}
      fullScreenOnPhone
      footer={<DialogFooter dialog={dialog} formId={formId} onClose={onClose} />}
    >
      <form
        id={formId}
        noValidate
        onSubmit={submit}
        className={dialog.primary ? "" : "grid gap-x-6 sm:grid-cols-2"}
      >
        <IdentityFields dialog={dialog} />
        {!dialog.primary && <AccessFields dialog={dialog} />}
      </form>
    </ModalDialog>
  );
}

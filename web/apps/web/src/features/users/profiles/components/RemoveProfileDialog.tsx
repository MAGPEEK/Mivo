import { useTranslation } from "react-i18next";

import { ConfirmDialog } from "../../../../shared/ui";
import { usersNamespace } from "../../locales";
import type { ProfileRemoval } from "../hooks/useProfileRemoval";

/** The "Delete profile?" confirmation; renders nothing while no profile is asked about. */
export function RemoveProfileDialog({ removal }: { readonly removal: ProfileRemoval }) {
  const { t } = useTranslation(usersNamespace);
  if (removal.target === null) {
    return null;
  }
  return (
    <ConfirmDialog
      title={t("profiles.remove.title")}
      description={t("profiles.remove.description", { name: removal.target.name })}
      confirmLabel={t("profiles.remove.confirm")}
      cancelLabel={t("users.remove.cancel")}
      pending={removal.pending}
      error={removal.errorText ?? undefined}
      onConfirm={removal.confirm}
      onCancel={removal.cancel}
    />
  );
}

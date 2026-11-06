import { useTranslation } from "react-i18next";

import { secondaryButtonClass } from "../../../shared/ui";
import { useSignOut } from "../hooks/useSignOut";

/** A "Sign out" button with its own failure message, e.g. on the profile picker. */
export function SignOutButton() {
  const { t } = useTranslation("auth");
  const action = useSignOut();
  return (
    <div>
      <button
        type="button"
        onClick={action.signOut}
        aria-disabled={action.pending}
        className={secondaryButtonClass}
      >
        {action.pending ? t("signOut.pending") : t("signOut.action")}
      </button>
      <p role="alert" className="mt-2 text-sm text-accent empty:hidden">
        {action.failed ? t("signOut.failed") : ""}
      </p>
    </div>
  );
}

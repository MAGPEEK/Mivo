import { useTranslation } from "react-i18next";

import { Skeleton } from "../../../shared/ui";
import { useSystemInfo } from "../hooks/useSystemInfo";

/**
 * Version of the connected server, for the sidebar footer. The status region exists in every
 * state, so screen readers announce when the version arrives or the server is unreachable.
 */
export function ServerVersion() {
  return (
    <div role="status" className="text-xs text-text-muted">
      <ServerVersionContent />
    </div>
  );
}

function ServerVersionContent() {
  const { t } = useTranslation();
  const systemInfo = useSystemInfo();
  if (systemInfo.isPending) {
    return <Skeleton label={t("system.version.loading")} className="h-4 w-28" />;
  }
  if (systemInfo.isError) {
    return t("system.unreachable");
  }
  return t("system.version.value", { version: systemInfo.data.version });
}

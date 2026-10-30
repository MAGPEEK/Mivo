import { useTranslation } from "react-i18next";

import { AlertIcon, Skeleton } from "../../../shared/ui";
import { useSystemInfo } from "../hooks/useSystemInfo";

/** Props of {@link ServerVersion}. */
export type ServerVersionProps =
  | { readonly compact?: false }
  | {
      /**
       * Layout for the collapsed sidebar, which has no room for text: the version is left to
       * screen readers and only an unreachable server stays visible, as an icon button.
       */
      readonly compact: true;
      /** Called when the compact error indicator is activated, to show the full text. */
      readonly onExpand: () => void;
    };

/**
 * Version of the connected server, for the sidebar footer. The status region exists in every
 * state, so screen readers announce when the version arrives or the server is unreachable.
 */
export function ServerVersion(props: ServerVersionProps) {
  return (
    <div role="status" className="text-xs text-text-muted">
      <ServerVersionContent {...props} />
    </div>
  );
}

function ServerVersionContent(props: ServerVersionProps) {
  const { t } = useTranslation();
  const systemInfo = useSystemInfo();
  if (systemInfo.isError) {
    return props.compact === true ? (
      <CompactUnreachableIndicator onExpand={props.onExpand} />
    ) : (
      <UnreachableIndicator />
    );
  }
  const label = systemInfo.isPending
    ? t("system.version.loading")
    : t("system.version.value", { version: systemInfo.data.version });
  if (props.compact === true) {
    return <span className="sr-only">{label}</span>;
  }
  return systemInfo.isPending ? <Skeleton label={label} className="h-4 w-28" /> : label;
}

function UnreachableIndicator() {
  const { t } = useTranslation();
  return (
    <span className="flex items-center gap-2">
      <AlertIcon className="size-4 shrink-0 text-accent" />
      <span>{t("system.unreachable")}</span>
    </span>
  );
}

interface CompactIndicatorProps {
  readonly onExpand: () => void;
}

// The icon carries the warning for sighted users. Its text, which is also the button's
// accessible name, appears as a tooltip on hover and on focus, so keyboard users see it too;
// activating the button (also by touch) expands the sidebar, where the text has room.
function CompactUnreachableIndicator({ onExpand }: CompactIndicatorProps) {
  const { t } = useTranslation();
  return (
    <button
      type="button"
      onClick={onExpand}
      className="group relative mx-auto grid size-10 place-items-center rounded-md text-accent hover:bg-surface-raised"
    >
      <AlertIcon className="size-5" />
      <span className="pointer-events-none absolute top-1/2 left-full z-10 ml-3 -translate-y-1/2 rounded-sm border border-divider bg-surface-raised px-2 py-1 text-xs whitespace-nowrap text-text opacity-0 shadow-overlay transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100 motion-reduce:transition-none">
        {t("system.unreachable")}
      </span>
    </button>
  );
}

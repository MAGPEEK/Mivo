import { useTranslation } from "react-i18next";

import { AlertIcon, Skeleton } from "../../../shared/ui";
import { useDismissibleTooltip } from "../hooks/useDismissibleTooltip";
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
      /** Id of the collapsed sidebar the indicator expands, for `aria-controls`. */
      readonly sidebarId: string;
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
      <CompactUnreachableIndicator onExpand={props.onExpand} sidebarId={props.sidebarId} />
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
  readonly sidebarId: string;
}

// The icon carries the warning for sighted users. Its text, which is also the button's
// accessible name, appears as a tooltip on hover and on focus, so keyboard users see it too;
// activating the button (also by touch) expands the sidebar, where the text has room.
// The tooltip stays in the DOM while closed, only transparent, so the name never goes away.
// An invisible ::before, a bit wider than the margin, bridges the gap to the icon, so the
// pointer can move onto the text without closing it; while closed the tooltip ignores the
// pointer.
function CompactUnreachableIndicator({ onExpand, sidebarId }: CompactIndicatorProps) {
  const { t } = useTranslation();
  const tooltip = useDismissibleTooltip();
  return (
    <button
      type="button"
      aria-expanded={false}
      aria-controls={sidebarId}
      onClick={onExpand}
      {...tooltip.triggerHandlers}
      className="relative mx-auto grid size-10 place-items-center rounded-md text-accent hover:bg-surface-raised"
    >
      <AlertIcon className="size-5" />
      <span
        data-state={tooltip.open ? "open" : "closed"}
        className="pointer-events-none absolute top-1/2 left-full z-10 ml-3 -translate-y-1/2 rounded-sm border border-divider bg-surface-raised px-2 py-1 text-xs whitespace-nowrap text-text opacity-0 shadow-overlay transition-opacity before:absolute before:inset-y-0 before:right-full before:w-4 data-[state=open]:pointer-events-auto data-[state=open]:opacity-100 motion-reduce:transition-none"
      >
        {t("system.unreachable")}
      </span>
    </button>
  );
}

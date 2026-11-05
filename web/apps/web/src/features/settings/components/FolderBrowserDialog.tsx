import type { FolderEntry, FolderRoot } from "@mivo/api-client";
import { useId } from "react";
import { useTranslation } from "react-i18next";

import { ModalDialog, primaryButtonClass, secondaryButtonClass } from "../../../shared/ui";
import { useFolderBrowser, type FolderBrowser } from "../hooks/useFolderBrowser";
import { useManagementErrorText } from "../hooks/useManagementErrorText";
import { joinDisplayPath } from "../lib/libraryDraft";
import { settingsNamespace } from "../locales";
import { PermissionHelp } from "./PermissionHelp";

/** Props of {@link FolderBrowserDialog}. */
export interface FolderBrowserDialogProps {
  /** Called with the root and the folder relative to it when the user chooses a folder. */
  readonly onSelect: (root: FolderRoot, relativePath: string) => void;
  readonly onClose: () => void;
}

/** The part of the dialog body that scrolls; the body itself does not (`body="fill"`). */
const scrollRegionClass = "-mx-1 min-h-0 flex-1 overflow-y-auto px-1 py-1";

function RootPicker({ browser }: { readonly browser: FolderBrowser }) {
  const { t } = useTranslation(settingsNamespace);
  const selectId = useId();
  const roots = browser.roots.data?.roots ?? [];
  return (
    <div className="mb-3 flex flex-col gap-1">
      <label htmlFor={selectId} className="text-sm font-medium">
        {t("libraries.browser.root")}
      </label>
      <select
        id={selectId}
        value={browser.root?.id ?? ""}
        onChange={(event) => {
          browser.chooseRoot(event.target.value);
        }}
        className="rounded-md border border-divider bg-surface px-3 py-2 text-sm"
      >
        {roots.map((root) => (
          <option key={root.id} value={root.id} disabled={!root.available}>
            {root.available
              ? root.path
              : t("libraries.browser.rootUnavailable", { path: root.path })}
          </option>
        ))}
      </select>
    </div>
  );
}

function Breadcrumb({ browser }: { readonly browser: FolderBrowser }) {
  const { t } = useTranslation(settingsNamespace);
  const last = browser.crumbs.length - 1;
  return (
    <div className="mb-3 flex items-center gap-3">
      <button
        type="button"
        disabled={browser.path === ""}
        onClick={browser.up}
        className={secondaryButtonClass}
      >
        {t("libraries.browser.up")}
      </button>
      <nav aria-label={t("libraries.breadcrumb.label")} className="min-w-0 text-sm">
        <ol className="flex flex-wrap items-center gap-1">
          {browser.crumbs.map((crumb, index) => (
            <li key={crumb.path} className="flex items-center gap-1">
              {index > 0 && <span aria-hidden="true">›</span>}
              {index === last ? (
                <span aria-current="page" className="break-all font-medium">
                  {crumb.label}
                </span>
              ) : (
                <button
                  type="button"
                  onClick={() => {
                    browser.goTo(crumb.path);
                  }}
                  className="break-all rounded-sm text-accent underline"
                >
                  {crumb.label}
                </button>
              )}
            </li>
          ))}
        </ol>
      </nav>
    </div>
  );
}

function EntryRow({ entry, onOpen }: { readonly entry: FolderEntry; readonly onOpen: () => void }) {
  const { t } = useTranslation(settingsNamespace);
  const reasonId = useId();
  const reason =
    entry.reason === "in_use"
      ? t("libraries.browser.reason.in_use")
      : t("libraries.browser.reason.reserved");
  return (
    <li className="flex flex-wrap items-center gap-2">
      {/* aria-disabled keeps the entry reachable by keyboard so its reason can be read. */}
      <button
        type="button"
        aria-disabled={!entry.selectable}
        aria-describedby={entry.selectable ? undefined : reasonId}
        onClick={entry.selectable ? onOpen : undefined}
        className="w-full rounded-md px-3 py-2 text-left text-sm wrap-anywhere hover:bg-surface aria-disabled:cursor-not-allowed aria-disabled:text-text-muted"
      >
        <span aria-hidden="true">📁 </span>
        {entry.name}
        {!entry.selectable && (
          <span id={reasonId} className="ml-2 text-xs text-text-muted">
            {reason}
          </span>
        )}
      </button>
    </li>
  );
}

function Listing({ browser }: { readonly browser: FolderBrowser }) {
  const { t } = useTranslation(settingsNamespace);
  const errorText = useManagementErrorText(browser.listing.error, "libraries.browser.loadFailed");
  const listing = browser.listing.data;
  const installKind = browser.roots.data?.installKind ?? "standard";
  const serviceUser = browser.roots.data?.serviceUser ?? "";
  if (errorText !== null) {
    return <p role="alert">{errorText}</p>;
  }
  if (listing === undefined) {
    return <p role="status">{t("libraries.browser.loading")}</p>;
  }
  // Only this region scrolls, so location, breadcrumb and "Select this folder" stay in view
  // however many folders there are (FIX-folder-select). The padding keeps focus rings visible.
  return (
    <div className={scrollRegionClass}>
      {listing.folders.length === 0 ? (
        <p className="text-sm text-text-muted">{t("libraries.browser.empty")}</p>
      ) : (
        <ul aria-label={t("libraries.browser.listLabel")} className="flex flex-col">
          {listing.folders.map((entry) => (
            <EntryRow
              key={entry.path}
              entry={entry}
              onOpen={() => {
                browser.goTo(entry.path);
              }}
            />
          ))}
        </ul>
      )}
      {listing.truncated && (
        <p className="mt-2 text-sm text-text-muted">{t("libraries.browser.truncated")}</p>
      )}
      {listing.unreadableCount > 0 && (
        <div role="status" className="mt-2 text-sm">
          <p>{t("libraries.browser.unreadable", { count: listing.unreadableCount })}</p>
          <PermissionHelp installKind={installKind} serviceUser={serviceUser} />
        </div>
      )}
    </div>
  );
}

function BrowserBody({ browser }: { readonly browser: FolderBrowser }) {
  const { t } = useTranslation(settingsNamespace);
  const errorText = useManagementErrorText(browser.roots.error, "libraries.browser.loadFailed");
  if (errorText !== null) {
    return <p role="alert">{errorText}</p>;
  }
  if (browser.roots.data === undefined) {
    return <p role="status">{t("libraries.browser.loading")}</p>;
  }
  if (browser.root === undefined) {
    return (
      <div className={scrollRegionClass}>
        <p>{t("libraries.browser.noRoots")}</p>
        <PermissionHelp
          installKind={browser.roots.data.installKind}
          serviceUser={browser.roots.data.serviceUser}
        />
      </div>
    );
  }
  return (
    <>
      <div className="shrink-0">
        <RootPicker browser={browser} />
        <Breadcrumb browser={browser} />
      </div>
      <Listing browser={browser} />
    </>
  );
}

/**
 * Second dialog of "Add folder": browses the folders below the server's folder roots. Unreadable
 * folders are not listed; the count and how to fix permissions are shown instead.
 */
export function FolderBrowserDialog({ onSelect, onClose }: FolderBrowserDialogProps) {
  const { t } = useTranslation(settingsNamespace);
  const browser = useFolderBrowser();
  const { root, path } = browser;
  return (
    <ModalDialog
      title={t("libraries.browser.title")}
      closeLabel={t("libraries.dialog.close")}
      onClose={onClose}
      size="wide"
      body="fill"
      fullScreenOnPhone
      footer={
        <>
          <span className="min-w-0 break-all font-mono text-sm text-text-muted">
            {root === undefined ? "" : joinDisplayPath(root.path, path)}
          </span>
          <span className="flex gap-3">
            <button type="button" onClick={onClose} className={secondaryButtonClass}>
              {t("libraries.dialog.cancel")}
            </button>
            <button
              type="button"
              aria-disabled={root === undefined}
              onClick={
                root === undefined
                  ? undefined
                  : () => {
                      onSelect(root, path);
                    }
              }
              className={primaryButtonClass}
            >
              {t("libraries.browser.select")}
            </button>
          </span>
        </>
      }
    >
      <p className="mb-3 shrink-0 text-sm text-text-muted">{t("libraries.browser.hint")}</p>
      <BrowserBody browser={browser} />
    </ModalDialog>
  );
}

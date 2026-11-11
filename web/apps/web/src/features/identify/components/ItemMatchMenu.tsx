import type { ItemDetail, ItemKind } from "@mivo/api-client";
import { useTranslation } from "react-i18next";

import { ConfirmDialog, Menu, type MenuEntry } from "../../../shared/ui";
import { useIdentifyErrorText, useOutcomeText } from "../hooks/useIdentifyTexts";
import { useMatchActions, type MatchActions } from "../hooks/useMatchActions";
import { isMatchableKind } from "../lib/searchInput";
import { identifyNamespace } from "../locales";
import { IdentifyDialog } from "./IdentifyDialog";

/** Props of {@link ItemMatchMenu}. */
export interface ItemMatchMenuProps {
  readonly itemId: string;
  readonly kind: ItemKind;
  readonly title: string;
  /** Year such as `2008`; prefills the search, undefined when unknown. */
  readonly year: string | undefined;
  /** Called after the item changed on the server, with its new state when the server sent it. */
  readonly onItemChanged: (item?: ItemDetail) => void;
  /** Entries of the page shown before the match actions, e.g. "Mark as watched". */
  readonly extraEntries?: readonly MenuEntry[];
}

/**
 * The detail page's overflow menu with "Correct match…", "Remove match" and "Refresh
 * metadata", and the dialogs behind them. Renders nothing for seasons, episodes and extras:
 * they follow their show or parent, and the page shows its own actions as buttons there. Like library management it is shown to everyone, and the
 * server refuses peers outside the local network (ADR 0028).
 */
export function ItemMatchMenu(props: ItemMatchMenuProps) {
  if (!isMatchableKind(props.kind)) {
    return null;
  }
  return <MatchMenu {...props} />;
}

function MatchMenu({ itemId, title, year, onItemChanged, extraEntries = [] }: ItemMatchMenuProps) {
  const { t } = useTranslation(identifyNamespace);
  const actions = useMatchActions({ itemId, onItemChanged });
  const outcomeText = useOutcomeText(actions.outcome);
  const entries: MenuEntry[] = [
    ...extraEntries,
    { id: "identify", label: t("menu.identify"), onSelect: actions.openIdentify },
    { id: "remove", label: t("menu.removeMatch"), onSelect: actions.openRemove },
    {
      id: "refresh",
      label: t("menu.refresh"),
      disabled: actions.refreshPending,
      onSelect: actions.refresh,
    },
  ];
  return (
    <div className="flex flex-wrap items-center gap-3">
      <Menu
        trigger={<span aria-hidden="true">⋮</span>}
        ariaLabel={t("menu.label")}
        entries={entries}
      />
      <p role="status" className="text-sm text-text-muted empty:hidden">
        {outcomeText}
      </p>
      {actions.dialog === "identify" && (
        <IdentifyDialog
          itemId={itemId}
          title={title}
          year={year}
          onApplied={actions.applied}
          onClose={actions.closeDialog}
        />
      )}
      {actions.dialog === "remove" && <RemoveMatchDialog title={title} actions={actions} />}
    </div>
  );
}

interface RemoveMatchDialogProps {
  readonly title: string;
  readonly actions: MatchActions;
}

function RemoveMatchDialog({ title, actions }: RemoveMatchDialogProps) {
  const { t } = useTranslation(identifyNamespace);
  const error = useIdentifyErrorText(actions.removeErrorKind);
  return (
    <ConfirmDialog
      title={t("remove.title")}
      description={t("remove.description", { title })}
      confirmLabel={t("remove.confirm")}
      cancelLabel={t("remove.cancel")}
      onConfirm={actions.confirmRemove}
      onCancel={actions.closeDialog}
      pending={actions.removePending}
      error={error}
    />
  );
}

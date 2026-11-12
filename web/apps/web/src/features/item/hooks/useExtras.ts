import type { ExtraType, ItemSummary } from "@mivo/api-client";
import { useTranslation } from "react-i18next";

import { groupExtras } from "../lib/extras";
import { itemNamespace } from "../locales";

/** A row of extras of one kind, with its translated heading. */
export interface ExtraRowView {
  readonly type: ExtraType;
  readonly title: string;
  readonly extras: readonly ItemSummary[];
}

/** The extras of an item grouped by kind, each group with its heading in the UI language. */
export function useExtras(extras: readonly ItemSummary[]): ExtraRowView[] {
  const { t } = useTranslation(itemNamespace);
  const headings: Readonly<Record<ExtraType, string>> = {
    trailer: t("extras.type.trailer"),
    featurette: t("extras.type.featurette"),
    behind_the_scenes: t("extras.type.behind_the_scenes"),
    deleted_scene: t("extras.type.deleted_scene"),
    interview: t("extras.type.interview"),
    scene: t("extras.type.scene"),
    short: t("extras.type.short"),
    other: t("extras.type.other"),
  };
  return groupExtras(extras).map((group) => ({ ...group, title: headings[group.type] }));
}

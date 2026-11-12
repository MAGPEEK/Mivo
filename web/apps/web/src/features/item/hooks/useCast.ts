import type { ItemPerson, PersonRole } from "@mivo/api-client";
import { useTranslation } from "react-i18next";

import { toCastRow, type CastMember } from "../lib/castRow";
import { itemNamespace } from "../locales";

/** The people of an item as cards; the role of someone without a character is translated. */
export function useCast(people: readonly ItemPerson[]): CastMember[] {
  const { t } = useTranslation(itemNamespace);
  const roles: Readonly<Record<PersonRole, string>> = {
    actor: t("cast.role.actor"),
    director: t("cast.role.director"),
    writer: t("cast.role.writer"),
    producer: t("cast.role.producer"),
    guest_star: t("cast.role.guest_star"),
    author: t("cast.role.author"),
    narrator: t("cast.role.narrator"),
  };
  return toCastRow(people, (role) => roles[role]);
}

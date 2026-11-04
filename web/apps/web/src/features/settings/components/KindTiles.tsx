import type { LibraryKind } from "@mivo/api-client";
import type { TFunction } from "i18next";
import { useId } from "react";
import { useTranslation } from "react-i18next";

import { LibraryKindIcon } from "../../library";
import { settingsNamespace } from "../locales";

/** Props of {@link KindTiles}. */
export interface KindTilesProps {
  /** The kinds the server accepts now (`systemInfo.libraryKinds`). */
  readonly kinds: readonly LibraryKind[];
  readonly value: LibraryKind;
  readonly onChange: (kind: LibraryKind) => void;
  /** An existing library cannot change its kind. */
  readonly locked: boolean;
  /** Error about the kind, shown below the tiles. */
  readonly error?: string | undefined;
}

// Later API versions may add kinds; they show their raw name instead of failing.
function kindName(kind: LibraryKind, t: TFunction<typeof settingsNamespace>): string {
  switch (kind) {
    case "movies":
      return t("libraries.kind.movies");
    case "shows":
      return t("libraries.kind.shows");
    default:
      return kind;
  }
}

/** Radio group of tiles, one per supported library kind. */
export function KindTiles({ kinds, value, onChange, locked, error }: KindTilesProps) {
  const { t } = useTranslation(settingsNamespace);
  const hintId = useId();
  const groupName = useId();
  return (
    <fieldset aria-describedby={hintId} className="mb-4">
      <legend className="mb-2 text-sm font-medium">{t("libraries.dialog.kind")}</legend>
      <div className="flex flex-wrap gap-3">
        {kinds.map((kind) => (
          <label
            key={kind}
            className="flex min-w-28 cursor-pointer flex-col items-center gap-1 rounded-lg border border-divider bg-surface p-3 text-sm has-[:checked]:border-accent has-[:focus-visible]:outline has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-accent has-[:disabled]:cursor-not-allowed has-[:disabled]:opacity-60"
          >
            <input
              type="radio"
              name={groupName}
              value={kind}
              checked={kind === value}
              disabled={locked}
              onChange={() => {
                onChange(kind);
              }}
              className="sr-only"
            />
            <LibraryKindIcon kind={kind} className="size-8 text-accent" />
            {kindName(kind, t)}
          </label>
        ))}
      </div>
      <p id={hintId} className="mt-2 text-xs text-text-muted">
        {locked ? t("libraries.dialog.kindLocked") : t("libraries.dialog.kindHint")}
      </p>
      <p role="alert" className="mt-1 text-sm text-accent empty:hidden">
        {error}
      </p>
    </fieldset>
  );
}

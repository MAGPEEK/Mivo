import { useId, type Ref, type SubmitEvent } from "react";
import { useTranslation } from "react-i18next";

import { searchShortcutAriaKeys } from "../lib/searchShortcut";
import { SearchIcon } from "./icons";

/** Props of {@link TopBar}. */
export interface TopBarProps {
  /** Receives the search field, so a global shortcut can focus it. */
  readonly searchInputRef: Ref<HTMLInputElement>;
}

// Search has no results page yet; without this, submitting would reload the app.
function ignoreSubmit(event: SubmitEvent<HTMLFormElement>) {
  event.preventDefault();
}

/** Bar above the page content with the global search. */
export function TopBar({ searchInputRef }: TopBarProps) {
  const { t } = useTranslation();
  const searchInputId = useId();
  return (
    <header className="sticky top-0 z-10 flex h-14 shrink-0 items-center border-b border-divider bg-background px-6">
      <form role="search" onSubmit={ignoreSubmit} className="relative w-full max-w-md">
        <label htmlFor={searchInputId} className="sr-only">
          {t("search.label")}
        </label>
        <SearchIcon className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-text-muted" />
        <input
          ref={searchInputRef}
          id={searchInputId}
          type="search"
          autoComplete="off"
          placeholder={t("search.placeholder")}
          aria-keyshortcuts={searchShortcutAriaKeys}
          className="h-9 w-full rounded-md border border-divider bg-surface pr-3 pl-9 text-sm text-text placeholder:text-text-muted hover:border-text-muted"
        />
      </form>
    </header>
  );
}

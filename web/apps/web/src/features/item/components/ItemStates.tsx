import { Link } from "@tanstack/react-router";
import type { Ref } from "react";
import { useTranslation } from "react-i18next";

import { useDeferredAnnouncement } from "../hooks/useDeferredAnnouncement";
import type { LoadFailure } from "../hooks/useItemDetails";
import { itemNamespace } from "../locales";

const placeholderShapeClass =
  "block animate-pulse rounded-sm bg-surface-raised motion-reduce:animate-none";

/**
 * Shown while the item loads. The heading keeps the page navigable by headings; the status
 * region mounts empty and receives its text right after, so screen readers announce it.
 */
export function ItemLoading() {
  const { t } = useTranslation(itemNamespace);
  const announcement = useDeferredAnnouncement(t("loading"));
  return (
    <>
      <h1 className="sr-only">{t("heading")}</h1>
      <div role="status" className="sr-only">
        {announcement}
      </div>
      <div aria-hidden="true" className="flex flex-col gap-4 sm:flex-row">
        <span className={`aspect-2/3 w-40 rounded-md ${placeholderShapeClass}`} />
        <span className="flex flex-1 flex-col gap-3">
          <span className={`h-9 w-2/3 ${placeholderShapeClass}`} />
          <span className={`h-5 w-1/3 ${placeholderShapeClass}`} />
        </span>
      </div>
    </>
  );
}

/** Shown when the server knows no item with this ID. */
export function ItemNotFound() {
  const { t } = useTranslation(itemNamespace);
  return (
    <>
      <h1 className="text-3xl font-semibold">{t("notFound.heading")}</h1>
      <p className="mt-2 text-text-muted">{t("notFound.message")}</p>
      <Link
        to="/"
        className="mt-6 inline-block rounded-md bg-accent px-4 py-2 font-medium text-on-accent hover:bg-accent-hover"
      >
        {t("notFound.backHome")}
      </Link>
    </>
  );
}

/** Props of {@link ItemLoadError}. */
export interface ItemLoadErrorProps {
  /** Receives the heading, so focus can move to it after a retry. */
  readonly headingRef: Ref<HTMLHeadingElement>;
  readonly failure: LoadFailure;
  /** Loads the item again; the page shows the loading state meanwhile. */
  readonly onRetry: () => void;
}

/** Shown when the item could not be loaded: the server is unreachable or answered an error. */
export function ItemLoadError({ headingRef, failure, onRetry }: ItemLoadErrorProps) {
  const { t } = useTranslation(itemNamespace);
  return (
    <>
      <h1
        ref={headingRef}
        tabIndex={-1}
        className="text-3xl font-semibold focus-visible:outline-none"
      >
        {t("loadError.heading")}
      </h1>
      <p role="alert" className="mt-2 text-text-muted">
        {failure === "network" ? t("loadError.network") : t("loadError.server")}
      </p>
      <button
        type="button"
        onClick={onRetry}
        className="mt-6 rounded-md bg-accent px-4 py-2 font-medium text-on-accent hover:bg-accent-hover"
      >
        {t("loadError.retry")}
      </button>
    </>
  );
}

import { secondaryButtonClass } from "../../../shared/ui";
import type { PagedItems } from "../hooks/pagedItems";

/** The translated texts of a {@link LoadMoreButton}. */
export interface LoadMoreTexts {
  readonly loadMore: string;
  readonly loadingMore: string;
  /** Shown above the button after a failed page. */
  readonly failed: string;
  readonly retry: string;
}

/** Props of {@link LoadMoreButton}. */
export interface LoadMoreButtonProps {
  readonly paged: PagedItems;
  readonly texts: LoadMoreTexts;
}

function buttonLabel(paged: PagedItems, texts: LoadMoreTexts): string {
  if (paged.loadingMore) {
    return texts.loadingMore;
  }
  return paged.loadMoreFailed ? texts.retry : texts.loadMore;
}

/** "Load more" below a paged list, with the error of a failed page above it. */
export function LoadMoreButton({ paged, texts }: LoadMoreButtonProps) {
  return (
    <div className="mt-8 flex flex-col items-center gap-3">
      {paged.loadMoreFailed && !paged.loadingMore && (
        <p role="alert" className="text-sm text-text-muted">
          {texts.failed}
        </p>
      )}
      {/* aria-disabled instead of disabled: a disabled button drops keyboard focus. */}
      <button
        type="button"
        onClick={paged.loadMore}
        aria-disabled={paged.loadingMore}
        aria-busy={paged.loadingMore}
        className={secondaryButtonClass}
      >
        {buttonLabel(paged, texts)}
      </button>
    </div>
  );
}

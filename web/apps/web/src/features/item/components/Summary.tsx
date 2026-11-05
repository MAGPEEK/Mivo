import { useId } from "react";
import { useTranslation } from "react-i18next";

import { useClampedText } from "../hooks/useClampedText";
import { itemNamespace } from "../locales";

/** Props of {@link Summary}. */
export interface SummaryProps {
  readonly tagline: string | undefined;
  readonly summary: string | undefined;
}

const toggleClass =
  "self-start rounded-sm text-sm font-medium text-accent hover:text-accent-hover hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent";

/**
 * The tagline and the summary clamped to four lines, with "More"/"Less" when the clamp cuts
 * the text (DT-06). Renders nothing when both are missing.
 */
export function Summary({ tagline, summary }: SummaryProps) {
  if (tagline === undefined && summary === undefined) {
    return null;
  }
  return (
    <div className="flex max-w-prose flex-col gap-1">
      {tagline !== undefined && <p className="text-text-muted italic">{tagline}</p>}
      {summary !== undefined && <ClampedSummary text={summary} />}
    </div>
  );
}

function ClampedSummary({ text }: { readonly text: string }) {
  const { t } = useTranslation(itemNamespace);
  const id = useId();
  const { ref, expanded, showToggle, toggle } = useClampedText(text);
  return (
    <>
      <p
        ref={ref}
        id={id}
        data-clamped={expanded ? undefined : ""}
        className={expanded ? "" : "line-clamp-4"}
      >
        {text}
      </p>
      {showToggle && (
        <button
          type="button"
          aria-expanded={expanded}
          aria-controls={id}
          onClick={toggle}
          className={toggleClass}
        >
          {expanded ? t("summaryToggle.less") : t("summaryToggle.more")}
        </button>
      )}
    </>
  );
}

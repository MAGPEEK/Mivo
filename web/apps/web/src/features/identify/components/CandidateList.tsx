import type { IdentifyCandidate } from "@mivo/api-client";
import { useTranslation } from "react-i18next";

import { Badge, FilmIcon, primaryButtonClass } from "../../../shared/ui";
import { candidateKey } from "../hooks/useIdentify";
import { candidatePosterSrc, candidateTitle } from "../lib/candidate";
import { providerName } from "../lib/searchInput";
import { identifyNamespace } from "../locales";

/** Props of {@link CandidateList}. */
export interface CandidateListProps {
  readonly candidates: readonly IdentifyCandidate[];
  /** Key of the candidate being applied; every Apply button is busy meanwhile. */
  readonly applyingKey: string | undefined;
  readonly onApply: (candidate: IdentifyCandidate) => void;
}

/** The candidates of an identify search, best first, each with an Apply button. */
export function CandidateList({ candidates, applyingKey, onApply }: CandidateListProps) {
  const { t } = useTranslation(identifyNamespace);
  return (
    <ul aria-label={t("dialog.results")} className="flex flex-col gap-3">
      {candidates.map((candidate) => {
        const key = candidateKey(candidate);
        return (
          <CandidateRow
            key={key}
            candidate={candidate}
            applying={applyingKey === key}
            busy={applyingKey !== undefined}
            onApply={() => {
              onApply(candidate);
            }}
          />
        );
      })}
    </ul>
  );
}

// Decorative: the title next to it names the candidate.
function CandidatePoster({ src }: { readonly src: string | undefined }) {
  if (src === undefined) {
    return (
      <div
        aria-hidden="true"
        className="grid aspect-2/3 w-14 shrink-0 place-items-center rounded-sm bg-surface-raised text-text-muted"
      >
        <FilmIcon className="size-6" />
      </div>
    );
  }
  return (
    <img
      src={src}
      alt=""
      loading="lazy"
      className="aspect-2/3 w-14 shrink-0 rounded-sm bg-surface-raised object-cover"
    />
  );
}

interface CandidateRowProps {
  readonly candidate: IdentifyCandidate;
  readonly applying: boolean;
  readonly busy: boolean;
  readonly onApply: () => void;
}

function CandidateRow({ candidate, applying, busy, onApply }: CandidateRowProps) {
  const { t } = useTranslation(identifyNamespace);
  return (
    <li className="flex items-start gap-3 rounded-md border border-divider bg-surface p-3">
      <CandidatePoster src={candidatePosterSrc(candidate)} />
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <h3 className="font-medium break-words text-text">
          {candidate.year === undefined ? (
            candidate.title
          ) : (
            <>
              {`${candidate.title} `}
              <span className="font-normal text-text-muted">({candidate.year})</span>
            </>
          )}
        </h3>
        <div>
          <Badge>{providerName(candidate.provider)}</Badge>
        </div>
        {candidate.overview !== undefined && candidate.overview !== "" && (
          <p className="line-clamp-2 text-sm text-text-muted">{candidate.overview}</p>
        )}
      </div>
      {/* aria-disabled instead of disabled: a disabled button drops keyboard focus. */}
      <button
        type="button"
        aria-label={t("candidate.applyLabel", { title: candidateTitle(candidate) })}
        aria-disabled={busy}
        onClick={busy ? undefined : onApply}
        className={`${primaryButtonClass} shrink-0`}
      >
        {applying ? t("candidate.applying") : t("candidate.apply")}
      </button>
    </li>
  );
}

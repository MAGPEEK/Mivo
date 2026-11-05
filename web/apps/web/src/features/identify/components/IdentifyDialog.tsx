import type { ItemDetail } from "@mivo/api-client";
import { useId, type SyntheticEvent } from "react";
import { useTranslation } from "react-i18next";

import { ModalDialog, primaryButtonClass, secondaryButtonClass } from "../../../shared/ui";
import { useIdentify, type IdentifyResults, type IdentifyState } from "../hooks/useIdentify";
import { useIdentifyErrorText } from "../hooks/useIdentifyTexts";
import { isRetryable } from "../lib/identifyError";
import { identifyNamespace } from "../locales";
import { CandidateList } from "./CandidateList";

/** Props of {@link IdentifyDialog}. */
export interface IdentifyDialogProps {
  readonly itemId: string;
  readonly title: string;
  readonly year: string | undefined;
  readonly onApplied: (item: ItemDetail) => void;
  readonly onClose: () => void;
}

const inputClass =
  "w-full rounded-md border border-divider bg-surface px-3 py-2 text-sm text-text placeholder:text-text-muted";
const labelClass = "text-sm font-medium text-text";

/**
 * "Correct match" dialog: search by title and year or by a pasted provider link, then apply a
 * candidate. The search runs at once with the item's title and year.
 */
export function IdentifyDialog({ itemId, title, year, onApplied, onClose }: IdentifyDialogProps) {
  const { t } = useTranslation(identifyNamespace);
  const state = useIdentify({ itemId, title, year, onApplied, onClose });
  const applyError = useIdentifyErrorText(state.applyErrorKind);
  const applying = state.applyingKey !== undefined;
  return (
    <ModalDialog
      title={t("dialog.title")}
      closeLabel={t("dialog.close")}
      onClose={state.close}
      footer={
        // aria-disabled keeps focus on the button while the match is applied.
        <button
          type="button"
          aria-disabled={applying}
          onClick={state.close}
          className={`ml-auto ${secondaryButtonClass}`}
        >
          {t("dialog.cancel")}
        </button>
      }
    >
      <SearchForm state={state} />
      <p role="status" className="mt-4 text-sm text-text-muted empty:hidden">
        {applying ? t("candidate.applyingStatus") : ""}
      </p>
      <p role="alert" className="mt-4 text-sm text-accent empty:hidden">
        {applyError}
      </p>
      <div className="mt-4">
        <Results state={state} />
      </div>
    </ModalDialog>
  );
}

function SearchForm({ state }: { readonly state: IdentifyState }) {
  const { t } = useTranslation(identifyNamespace);
  const queryId = useId();
  const hintId = useId();
  const yearId = useId();
  const onSubmit = (event: SyntheticEvent) => {
    event.preventDefault();
    state.submit();
  };
  return (
    <form role="search" onSubmit={onSubmit} className="flex flex-col gap-2">
      <div className="flex flex-wrap items-end gap-3">
        <div className="flex min-w-48 flex-1 flex-col gap-1">
          <label htmlFor={queryId} className={labelClass}>
            {t("dialog.query")}
          </label>
          <input
            id={queryId}
            type="search"
            data-autofocus
            aria-describedby={hintId}
            value={state.query}
            onChange={(event) => {
              state.setQuery(event.target.value);
            }}
            className={inputClass}
          />
        </div>
        <YearField id={yearId} value={state.year} onChange={state.setYear} />
        <button type="submit" className={primaryButtonClass}>
          {t("dialog.search")}
        </button>
      </div>
      <p id={hintId} className="text-xs text-text-muted">
        {t("dialog.queryHint")}
      </p>
    </form>
  );
}

interface YearFieldProps {
  readonly id: string;
  readonly value: string;
  readonly onChange: (year: string) => void;
}

function YearField({ id, value, onChange }: YearFieldProps) {
  const { t } = useTranslation(identifyNamespace);
  return (
    <div className="flex w-24 flex-col gap-1">
      <label htmlFor={id} className={labelClass}>
        {t("dialog.year")}
      </label>
      <input
        id={id}
        inputMode="numeric"
        maxLength={4}
        value={value}
        onChange={(event) => {
          onChange(event.target.value);
        }}
        className={inputClass}
      />
    </div>
  );
}

function Results({ state }: { readonly state: IdentifyState }) {
  const { t } = useTranslation(identifyNamespace);
  const { results } = state;
  return (
    <>
      <p role="status" className="text-sm text-text-muted empty:hidden">
        {resultStatusText(results, t("dialog.searching"), t("dialog.noResults"))}
      </p>
      {results.status === "error" && <SearchError state={state} results={results} />}
      {results.status === "success" && results.candidates.length > 0 && (
        <CandidateList
          candidates={results.candidates}
          applyingKey={state.applyingKey}
          onApply={state.apply}
        />
      )}
    </>
  );
}

function resultStatusText(results: IdentifyResults, searching: string, noResults: string) {
  if (results.status === "pending") {
    return searching;
  }
  return results.status === "success" && results.candidates.length === 0 ? noResults : "";
}

interface SearchErrorProps {
  readonly state: IdentifyState;
  readonly results: Extract<IdentifyResults, { status: "error" }>;
}

function SearchError({ state, results }: SearchErrorProps) {
  const { t } = useTranslation(identifyNamespace);
  const text = useIdentifyErrorText(results.errorKind);
  const retryable = isRetryable(results.errorKind);
  return (
    <div className="flex flex-wrap items-center gap-3">
      <p role="alert" className="text-sm text-text">
        {text}
      </p>
      {retryable && (
        <button type="button" onClick={state.submit} className={secondaryButtonClass}>
          {t("dialog.retry")}
        </button>
      )}
    </div>
  );
}

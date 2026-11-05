import { useId } from "react";
import { useTranslation } from "react-i18next";

import { Skeleton } from "../../../../shared/ui";
import { settingsNamespace } from "../../locales";
import type { MetadataProviderStatus } from "../api/metadataSettings";
import { useMetadataStatus } from "../hooks/useMetadataStatus";

// Literal keys, so a misspelled one fails type checking; values not listed read "Unknown".
const stateKeys = {
  ok: "metadata.status.state.ok",
  rate_limited: "metadata.status.state.rate_limited",
  offline: "metadata.status.state.offline",
  disabled: "metadata.status.state.disabled",
} as const;

const providerKeys = {
  tmdb: "metadata.status.provider.tmdb",
  musicbrainz: "metadata.status.provider.musicbrainz",
  coverartarchive: "metadata.status.provider.coverartarchive",
} as const;

// The dot repeats the state for sighted users; the text carries it for everyone.
const stateDotClasses = {
  ok: "bg-direct-play",
  rate_limited: "bg-transcode",
  offline: "bg-accent",
  disabled: "bg-text-muted",
};

// Servers may send values this client does not know (ADR 0017); those find no key.
function keyFor<Key extends string>(
  keys: Readonly<Record<string, Key>>,
  value: string,
): Key | undefined {
  return Object.hasOwn(keys, value) ? keys[value] : undefined;
}

function ProviderRow({ entry }: { readonly entry: MetadataProviderStatus }) {
  const { t } = useTranslation(settingsNamespace);
  const providerKey = keyFor(providerKeys, entry.provider);
  const stateKey = keyFor(stateKeys, entry.state);
  const name = providerKey === undefined ? entry.provider : t(providerKey);
  const state = stateKey === undefined ? t("metadata.status.state.unknown") : t(stateKey);
  return (
    <li className="flex items-center justify-between gap-4 py-1.5">
      <span>{name}</span>
      <span className="flex items-center gap-2 text-text-muted">
        <span
          aria-hidden="true"
          className={`size-2 rounded-full ${keyFor(stateDotClasses, entry.state) ?? "bg-text-muted"}`}
        />
        {state}
      </span>
    </li>
  );
}

function StatusContent() {
  const { t } = useTranslation(settingsNamespace);
  const load = useMetadataStatus();
  switch (load.status) {
    case "loading":
      return <Skeleton label={t("metadata.status.loading")} className="h-5 w-48" />;
    case "unavailable":
      return <p className="text-text-muted">{t("metadata.status.unavailable")}</p>;
    case "error":
      return <p className="text-text-muted">{t("metadata.status.failed")}</p>;
    case "ready":
      return (
        <>
          <p>{t("metadata.status.queue", { count: load.metadataStatus.queueLength })}</p>
          <ul
            aria-label={t("metadata.status.providersLabel")}
            className="mt-2 divide-y divide-divider"
          >
            {load.metadataStatus.providers.map((entry) => (
              <ProviderRow key={entry.provider} entry={entry} />
            ))}
          </ul>
        </>
      );
  }
}

/** Queue length and provider states, refreshed while the panel is shown. */
export function MetadataStatusPanel() {
  const { t } = useTranslation(settingsNamespace);
  const headingId = useId();
  return (
    <section
      aria-labelledby={headingId}
      className="rounded-lg border border-divider bg-surface p-4 text-sm"
    >
      <h2 id={headingId} className="mb-2 text-base font-semibold">
        {t("metadata.status.heading")}
      </h2>
      <StatusContent />
    </section>
  );
}

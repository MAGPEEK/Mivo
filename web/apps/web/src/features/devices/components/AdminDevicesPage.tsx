import { useId } from "react";
import { useTranslation } from "react-i18next";

import { useClientFilter } from "../hooks/useClientFilter";
import { clientFilters, type ClientFilter } from "../lib/devices";
import { devicesNamespace } from "../locales";
import { DevicesSection } from "./DevicesSection";

const filterKeys: Record<ClientFilter, `filter.all` | `client.${Exclude<ClientFilter, "all">}`> = {
  all: "filter.all",
  web: "client.web",
  tv: "client.tv",
  cli: "client.cli",
};

function ClientFilterSelect({
  filter,
  onSelect,
}: {
  readonly filter: ClientFilter;
  readonly onSelect: (value: string) => void;
}) {
  const { t } = useTranslation(devicesNamespace);
  const selectId = useId();
  return (
    <div className="mb-4 flex flex-wrap items-center gap-2">
      <label htmlFor={selectId} className="text-sm text-text-muted">
        {t("filter.label")}
      </label>
      <select
        id={selectId}
        value={filter}
        onChange={(event) => {
          onSelect(event.target.value);
        }}
        className="rounded-md border border-divider bg-surface px-3 py-1.5 text-sm text-text"
      >
        {clientFilters.map((value) => (
          <option key={value} value={value}>
            {t(filterKeys[value])}
          </option>
        ))}
      </select>
    </div>
  );
}

/** Admin › Devices: the devices of every account with a client-type filter (US-25). */
export function AdminDevicesPage() {
  const { t } = useTranslation(devicesNamespace);
  const clientFilter = useClientFilter();
  return (
    <div className="max-w-6xl">
      <nav aria-label={t("admin.breadcrumbLabel")} className="mb-2 text-sm text-text-muted">
        <ol className="flex gap-2">
          <li>{t("admin.breadcrumbSettings")}</li>
          <li aria-hidden="true">›</li>
          <li aria-current="page">{t("admin.heading")}</li>
        </ol>
      </nav>
      <h1 className="mb-2 text-3xl font-semibold">{t("admin.heading")}</h1>
      <p className="mb-6 text-sm text-text-muted">{t("admin.description")}</p>
      <ClientFilterSelect filter={clientFilter.filter} onSelect={clientFilter.select} />
      <DevicesSection
        scope="all"
        select={clientFilter.apply}
        filteredEmptyText={t("state.filteredEmpty")}
      />
    </div>
  );
}

import { useTranslation } from "react-i18next";

import { ServerRow } from "../../library";
import { useHeroPreference } from "../hooks/useHeroPreference";
import { useHomeRows, type HomeRowsState } from "../hooks/useHomeRows";
import { Hero } from "./Hero";
import { NoLibraries, RowsHint, RowsLoadError, RowsSkeleton } from "./HomeStates";

/**
 * Home page: the optional hero (HO-01) and one media row per server row of the pinned
 * libraries (HO-02, HO-07), or the "No libraries yet" state leading to Settings › Libraries
 * (HO-09).
 */
export function HomePage() {
  const { t } = useTranslation();
  const home = useHomeRows();
  return (
    <>
      <h1 className="mb-6 text-3xl font-semibold">{t("home.heading")}</h1>
      <HomeContent home={home} />
    </>
  );
}

function HomeContent({ home }: { readonly home: HomeRowsState }) {
  const heroPreference = useHeroPreference();
  switch (home.state) {
    case "noLibraries":
      return <NoLibraries />;
    case "loading":
      return <RowsSkeleton />;
    case "error":
      return <RowsLoadError onRetry={home.retry} />;
    default:
      return (
        <>
          {heroPreference.enabled && <Hero items={home.hero} />}
          <div className="flex flex-col gap-8">
            {home.rows.map((row) => (
              <ServerRow key={row.id} row={row} />
            ))}
            {home.hint !== undefined && <RowsHint hint={home.hint} />}
          </div>
        </>
      );
  }
}

import type { Row } from "@mivo/api-client";
import { Link } from "@tanstack/react-router";

import { MediaRow, useRowTitle, type SeeAllLinkProps } from "../../../shared/ui";
import { rowCardFormat, seeAllTarget } from "../lib/homeRows";
import { RowItemCard } from "./RowItemCard";

/** Props of {@link HomeRow}. */
export interface HomeRowProps {
  readonly row: Row;
}

/** One server row on Home: its translated title, its cards and "See all" into Browse. */
export function HomeRow({ row }: HomeRowProps) {
  const rowTitle = useRowTitle();
  const format = rowCardFormat(row.cardFormat);
  const target = seeAllTarget(row);
  const seeAll =
    target === undefined
      ? {}
      : {
          renderSeeAll: (linkProps: SeeAllLinkProps) => (
            <Link
              to="/libraries/$libraryId"
              params={{ libraryId: target.libraryId }}
              search={target.search}
              {...linkProps}
            />
          ),
        };
  return (
    <MediaRow
      title={rowTitle(row)}
      items={row.items}
      format={format}
      getKey={(item) => item.id}
      renderCard={(item, card) => <RowItemCard item={item} format={format} rowCard={card} />}
      {...seeAll}
    />
  );
}

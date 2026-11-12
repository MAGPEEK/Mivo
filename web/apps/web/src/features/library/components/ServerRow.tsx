import type { Row } from "@mivo/api-client";
import { Link } from "@tanstack/react-router";

import { MediaRow, useRowTitle, type SeeAllLinkProps } from "../../../shared/ui";
import { rowCardFormat, seeAllTarget } from "../lib/serverRows";
import { ContinueWatchingCard } from "./ContinueWatchingCard";
import { RowItemCard } from "./RowItemCard";

/** Props of {@link ServerRow}. */
export interface ServerRowProps {
  readonly row: Row;
}

/**
 * One row the server generated, on Home or a library's Overview pivot: its translated title,
 * its cards and "See all" into the Browse pivot of its library.
 */
export function ServerRow({ row }: ServerRowProps) {
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
      renderCard={(item, card) =>
        row.kind === "continueWatching" ? (
          <ContinueWatchingCard item={item} format={format} rowCard={card} />
        ) : (
          <RowItemCard item={item} format={format} rowCard={card} />
        )
      }
      {...seeAll}
    />
  );
}

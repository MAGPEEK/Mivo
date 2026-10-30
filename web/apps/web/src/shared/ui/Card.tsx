import { useId, type ReactNode } from "react";

/** What {@link MediaCardProps.renderLink} receives; spread it onto the link element. */
export interface CardLinkProps {
  readonly className: string;
  readonly children: ReactNode;
  /** Points at the card's details, so they are read after the link name. */
  readonly "aria-describedby": string;
}

/** Props of {@link MediaCard}. */
export interface MediaCardProps {
  /** Title of the card and the accessible name of its link. */
  readonly title: string;
  /** Large text on the placeholder poster (e.g. the title initial) until artwork exists. */
  readonly posterText: string;
  /**
   * Renders the link to the detail page with the given props, e.g. a router `Link`. It
   * stretches over the whole card, so the card is one click target with one tab stop.
   */
  readonly renderLink: (props: CardLinkProps) => ReactNode;
  /** Facts below the title, e.g. the duration and badges. */
  readonly details?: ReactNode;
  /** Greys the card out, e.g. for a file that cannot be played. */
  readonly muted?: boolean;
}

// The link covers the card with its ::after box (the "stretched link" card pattern): the
// title stays the link's whole name instead of every text in the card, and the focus ring
// is drawn on that box, around the card.
const stretchedLinkClass =
  "after:absolute after:inset-0 after:rounded-md focus-visible:outline-none focus-visible:after:outline-2 focus-visible:after:outline-offset-2 focus-visible:after:outline-accent";

/** Poster card (2:3) with title and details; hovering lifts it like the rest of the design. */
export function MediaCard({
  title,
  posterText,
  renderLink,
  details,
  muted = false,
}: MediaCardProps) {
  const detailsId = useId();
  return (
    <div className="group relative flex flex-col gap-2">
      <div
        aria-hidden="true"
        className={`grid aspect-2/3 place-items-center rounded-md bg-surface-raised text-4xl font-semibold text-text-muted shadow-card transition motion-safe:group-hover:-translate-y-1 motion-reduce:transition-none group-hover:shadow-card-hover ${muted ? "opacity-50" : ""}`}
      >
        {posterText}
      </div>
      <h2 className={`truncate text-sm font-medium ${muted ? "text-text-muted" : "text-text"}`}>
        {renderLink({
          className: stretchedLinkClass,
          children: title,
          "aria-describedby": detailsId,
        })}
      </h2>
      <div
        id={detailsId}
        className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-text-muted"
      >
        {details}
      </div>
    </div>
  );
}

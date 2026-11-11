import { useId, type ReactNode } from "react";

/** What {@link MediaCardProps.renderLink} receives; spread it onto the link element. */
export interface CardLinkProps {
  readonly className: string;
  readonly children: ReactNode;
  /** Points at the card's details, so they are read after the link name. */
  readonly "aria-describedby": string;
}

/** Level of the card title's heading: 2 in a page grid, 3 in a row below its own heading. */
export type CardHeadingLevel = 2 | 3;

/** Props of {@link MediaCard}. */
export interface MediaCardProps {
  /** Title of the card and the accessible name of its link. */
  readonly title: string;
  /**
   * The artwork, e.g. a `Poster`. It is hidden from assistive technology: the title link
   * already names the card.
   */
  readonly poster: ReactNode;
  /**
   * Renders the link to the detail page with the given props, e.g. a router `Link`. It
   * stretches over the whole card, so the card is one click target with one tab stop.
   */
  readonly renderLink: (props: CardLinkProps) => ReactNode;
  /** Facts below the title, e.g. the duration and badges. */
  readonly details?: ReactNode;
  /**
   * Badges or a progress bar drawn over the artwork (watched state, M7). Unlike the artwork it
   * stays visible to assistive technology, so it must label itself.
   */
  readonly overlay?: ReactNode;
  /**
   * Buttons shown in the top left corner of the artwork while the card is hovered or holds
   * focus, e.g. the watched toggle. They come after the link in the tab order.
   */
  readonly actions?: ReactNode;
  /** Greys the card out, e.g. for a file that cannot be played. */
  readonly muted?: boolean;
  readonly headingLevel?: CardHeadingLevel;
}

// The link covers the card with its ::after box (the "stretched link" card pattern): the
// title stays the link's whole name instead of every text in the card, and the focus ring
// is drawn on that box, around the card.
/** Classes that stretch a link over its positioned parent, so the whole card is one click target. */
export const stretchedLinkClass =
  "after:absolute after:inset-0 after:rounded-md focus-visible:outline-none focus-visible:after:outline-2 focus-visible:after:outline-offset-2 focus-visible:after:outline-accent";

const liftClass = "transition motion-safe:group-hover:-translate-y-1 motion-reduce:transition-none";

/** Artwork card with title and details; hovering lifts it like the rest of the design. */
export function MediaCard({
  title,
  poster,
  renderLink,
  details,
  overlay,
  actions,
  muted = false,
  headingLevel = 2,
}: MediaCardProps) {
  const detailsId = useId();
  const Heading = headingLevel === 2 ? "h2" : "h3";
  return (
    <div className="group relative flex flex-col gap-2">
      <div className="relative">
        <div
          aria-hidden="true"
          className={`overflow-hidden rounded-md shadow-card group-hover:shadow-card-hover ${liftClass} ${muted ? "opacity-50" : ""}`}
        >
          {poster}
        </div>
        {overlay !== undefined && (
          <div data-card-overlay="" className={`pointer-events-none absolute inset-0 ${liftClass}`}>
            {overlay}
          </div>
        )}
      </div>
      <Heading
        className={`truncate text-sm font-medium ${muted ? "text-text-muted" : "text-text"}`}
      >
        {renderLink({
          className: stretchedLinkClass,
          children: title,
          "aria-describedby": detailsId,
        })}
      </Heading>
      <div
        id={detailsId}
        className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-text-muted"
      >
        {details}
      </div>
      {actions !== undefined && <CardActions>{actions}</CardActions>}
    </div>
  );
}

// Last in the card's DOM, so the link stays its first tab stop; z-10 lifts the buttons above
// the link's stretched click area. Hidden, not transparent, until hover or focus: a tap on a
// touch screen must reach the link, never an invisible button.
function CardActions({ children }: { readonly children: ReactNode }) {
  return (
    <div
      data-card-actions=""
      className={`absolute top-1.5 left-1.5 z-10 hidden gap-1 group-focus-within:flex group-hover:flex ${liftClass}`}
    >
      {children}
    </div>
  );
}

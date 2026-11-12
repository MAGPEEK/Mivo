/**
 * Moves focus off a row card that is about to disappear: to the link of the next card in its
 * row, else the previous one, else the row's heading. `from` is any element inside the card.
 */
export function focusNeighbourOf(from: Element | null): void {
  const item = from?.closest("li");
  if (item === null || item === undefined) {
    return;
  }
  const neighbour = item.nextElementSibling ?? item.previousElementSibling;
  const link = neighbour?.querySelector<HTMLElement>("a[href]");
  if (link !== null && link !== undefined) {
    link.focus();
    return;
  }
  const heading = item.closest("section")?.querySelector<HTMLElement>("h2");
  if (heading !== null && heading !== undefined) {
    heading.tabIndex = -1;
    heading.focus();
  }
}

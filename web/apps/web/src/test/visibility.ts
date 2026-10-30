/**
 * Whether `element` is hidden visually but kept for screen readers, i.e. it sits inside an
 * `sr-only` element. jsdom applies no Tailwind CSS, so the class is the only signal.
 */
export function isVisuallyHidden(element: Element): boolean {
  return element.closest(".sr-only") !== null;
}

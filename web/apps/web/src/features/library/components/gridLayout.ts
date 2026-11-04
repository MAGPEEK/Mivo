// Dense poster rows like the rest of the design: more columns as the window widens.
/** Classes of the library poster grid. */
export const itemGridClass =
  "grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 xl:grid-cols-8";

// One column of the grid above per breakpoint (Tailwind's sm 640, md 768, lg 1024, xl 1280
// pixels). The page margins and the sidebar only make a column narrower, so the viewport share
// is an upper bound and the browser never picks a rendition that is too small. The size
// slider (LB-28, M5) will replace this fixed expression.
/** `sizes` of a poster in {@link itemGridClass}. */
export const itemPosterSizes =
  "(min-width: 1280px) 12.5vw, (min-width: 1024px) 17vw, (min-width: 768px) 25vw, (min-width: 640px) 34vw, 50vw";

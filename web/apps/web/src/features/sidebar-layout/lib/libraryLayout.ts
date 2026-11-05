// Pure ordering and merge rules of the sidebar's library layout (SH-12, SH-13, SH-16). The
// layout stores unpinned ids rather than pinned ones, so a library the layout has never seen
// is pinned without extra bookkeeping.

/** A user's arrangement of the libraries, as saved. */
export interface LibraryLayout {
  /** Library ids in the user's order. */
  readonly order: readonly string[];
  /** Ids the user unpinned; every other library is pinned. */
  readonly unpinned: readonly string[];
}

/** One library of the current server list, in the user's order. */
export interface ArrangedLibrary {
  readonly id: string;
  readonly pinned: boolean;
}

// Bumped when the stored shape changes; an older or unknown version falls back to defaults.
const layoutVersion = 1;

/**
 * Applies `layout` to the libraries the server has now: the saved order first, then libraries
 * the layout does not know, pinned, in server order. Ids the server no longer has drop out.
 * Without a layout the result is the server order with everything pinned.
 */
export function arrangeLibraries(
  serverIds: readonly string[],
  layout: LibraryLayout | undefined,
): ArrangedLibrary[] {
  const existing = new Set(serverIds);
  const ordered = new Set((layout?.order ?? []).filter((id) => existing.has(id)));
  for (const id of serverIds) {
    ordered.add(id);
  }
  const unpinned = new Set(layout?.unpinned);
  return [...ordered].map((id) => ({ id, pinned: !unpinned.has(id) }));
}

/** Moves a library by `offset` places; a move past either end changes nothing. */
export function moveLibraryBy(
  libraries: readonly ArrangedLibrary[],
  id: string,
  offset: number,
): ArrangedLibrary[] {
  const from = libraries.findIndex((library) => library.id === id);
  const to = from + offset;
  if (from < 0 || to < 0 || to >= libraries.length) {
    return [...libraries];
  }
  return moveIndex(libraries, from, to);
}

/** Moves a dragged library to the place of the library it was dropped on. */
export function moveLibraryTo(
  libraries: readonly ArrangedLibrary[],
  id: string,
  targetId: string,
): ArrangedLibrary[] {
  const from = libraries.findIndex((library) => library.id === id);
  const to = libraries.findIndex((library) => library.id === targetId);
  if (from < 0 || to < 0) {
    return [...libraries];
  }
  return moveIndex(libraries, from, to);
}

function moveIndex(libraries: readonly ArrangedLibrary[], from: number, to: number) {
  const result = [...libraries];
  const [moved] = result.splice(from, 1);
  if (moved !== undefined) {
    result.splice(to, 0, moved);
  }
  return result;
}

/** Pins an unpinned library and unpins a pinned one. */
export function toggleLibraryPinned(
  libraries: readonly ArrangedLibrary[],
  id: string,
): ArrangedLibrary[] {
  return libraries.map((library) =>
    library.id === id ? { id, pinned: !library.pinned } : library,
  );
}

/** The layout to save for an arrangement. */
export function toLayout(libraries: readonly ArrangedLibrary[]): LibraryLayout {
  return {
    order: libraries.map((library) => library.id),
    unpinned: libraries.filter((library) => !library.pinned).map((library) => library.id),
  };
}

/** Turns a layout into the versioned string kept in storage. */
export function serializeLayout(layout: LibraryLayout): string {
  return JSON.stringify({ version: layoutVersion, order: layout.order, unpinned: layout.unpinned });
}

/**
 * Reads a string written by {@link serializeLayout}. Anything else – missing, corrupt, a
 * legacy shape or another version – yields `undefined`, which means the default layout.
 */
export function parseStoredLayout(raw: string | null): LibraryLayout | undefined {
  if (raw === null || raw === "") {
    return undefined;
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return undefined;
  }
  if (typeof parsed !== "object" || parsed === null) {
    return undefined;
  }
  const { version, order, unpinned } = parsed as Record<string, unknown>;
  if (version !== layoutVersion || !isStringArray(order) || !isStringArray(unpinned)) {
    return undefined;
  }
  return { order, unpinned };
}

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((entry) => typeof entry === "string");
}

import { Link } from "@tanstack/react-router";
import { useEffect, useRef, useState, type DragEvent, type Ref } from "react";
import { useTranslation } from "react-i18next";

import {
  AlertIcon,
  ChevronDownIcon,
  ChevronUpIcon,
  LibraryIcon,
  PinIcon,
  ResetIcon,
} from "./icons";
import {
  labelClass,
  sidebarLinkClass,
  type LayoutAnnouncement,
  type SidebarLayoutControls,
  type SidebarLibraryLink,
} from "./sidebarParts";

const listClass = "mt-2 flex flex-col gap-1";
const smallButtonBaseClass =
  "grid shrink-0 place-items-center rounded-md text-text-muted hover:bg-surface-raised hover:text-text disabled:pointer-events-none disabled:opacity-30 aria-pressed:text-accent";

/** Drag and drop handlers that make a library link a reorderable row. */
interface DragHandlers {
  readonly draggable: true;
  readonly onDragStart: (event: DragEvent) => void;
  readonly onDragOver: (event: DragEvent) => void;
  readonly onDragLeave: () => void;
  readonly onDrop: (event: DragEvent) => void;
  readonly onDragEnd: () => void;
}

/**
 * Size of the rearranging controls: compact for the sidebar, touch for the phone sheet, where
 * every control is at least 44 px (min-h-11/size-11, WCAG 2.5.5).
 */
export type ArrangeVariant = "compact" | "touch";

const linkSizeClass: Record<ArrangeVariant, string> = { compact: "h-10", touch: "min-h-11" };
const smallButtonSizeClass: Record<ArrangeVariant, string> = {
  compact: "size-7",
  touch: "size-11",
};

function smallButtonClass(variant: ArrangeVariant): string {
  return `${smallButtonBaseClass} ${smallButtonSizeClass[variant]}`;
}

// The sidebar link classes with the height of the variant instead of the fixed sidebar height.
function entryClass(variant: ArrangeVariant): string {
  return sidebarLinkClass.replace("h-10", linkSizeClass[variant]);
}

/** Announces layout changes to screen readers; also repeats an identical message. */
export function LayoutLiveRegion({ announcement }: { readonly announcement: LayoutAnnouncement }) {
  return (
    // Not role="status": the sidebar footer's server state already is the status. A new child
    // per message makes screen readers read a repeated text again.
    <div aria-live="polite" aria-atomic="true" className="sr-only">
      {announcement.text !== "" && <span key={announcement.id}>{announcement.text}</span>}
    </div>
  );
}

interface LibraryLinkProps {
  readonly collapsed: boolean;
  readonly library: SidebarLibraryLink;
  readonly dragHandlers?: DragHandlers;
  readonly variant?: ArrangeVariant;
  /** Called when the link is followed, e.g. to close the sheet around it. */
  readonly onNavigate?: (() => void) | undefined;
}

/** Link to one library with its kind icon and, if set, its note. */
export function LibraryLink({
  collapsed,
  library,
  dragHandlers,
  variant = "compact",
  onNavigate,
}: LibraryLinkProps) {
  return (
    // The link itself is the drag source and drop target: it is the row's interactive element.
    <Link
      to="/libraries/$libraryId"
      params={{ libraryId: library.id }}
      {...dragHandlers}
      onClick={onNavigate}
      className={`${entryClass(variant)} relative min-w-0 flex-1 ${collapsed ? "justify-center" : ""}`}
    >
      {/* The icon draws in currentColor, so the wrapper's colour marks the current library. */}
      <span className="contents group-aria-[current=page]:text-accent">{library.icon}</span>
      <span className={labelClass(collapsed, "min-w-0 flex-1 truncate")}>{library.name}</span>
      {library.note !== undefined && (
        <>
          {/* Visible in both states: the only hint at an unavailable library while collapsed. */}
          <AlertIcon
            className={
              collapsed
                ? "absolute top-1 right-1 size-3.5 text-accent"
                : "size-4 shrink-0 text-accent"
            }
          />
          <span className="sr-only">{`, ${library.note}`}</span>
        </>
      )}
    </Link>
  );
}

/** Props of {@link PinnedLibraries}. */
export interface PinnedLibrariesProps {
  readonly collapsed: boolean;
  /** Pinned libraries in display order. */
  readonly links: readonly SidebarLibraryLink[];
  readonly layout: SidebarLayoutControls;
  readonly onShowAll: () => void;
  readonly showAllRef: Ref<HTMLButtonElement>;
}

/**
 * The pinned libraries, reordered by drag and drop (SH-13), and the "All libraries" entry
 * (SH-12). Keyboard users reorder with the buttons of the full list instead.
 */
export function PinnedLibraries({
  collapsed,
  links,
  layout,
  onShowAll,
  showAllRef,
}: PinnedLibrariesProps) {
  const { t } = useTranslation();
  const drag = useLibraryDrag(layout.moveTo);
  return (
    <>
      {links.length > 0 && (
        <ul className={listClass}>
          {links.map((library) => (
            <li key={library.id} className={`flex rounded-md ${drag.classFor(library.id)}`}>
              <LibraryLink
                collapsed={collapsed}
                library={library}
                dragHandlers={drag.handlersFor(library.id)}
              />
            </li>
          ))}
        </ul>
      )}
      <button
        type="button"
        ref={showAllRef}
        onClick={onShowAll}
        className={`${sidebarLinkClass} mt-1 w-full ${collapsed ? "justify-center" : ""}`}
      >
        <LibraryIcon className="size-5 shrink-0" />
        <span className={labelClass(collapsed, "truncate")}>
          {t("sidebar.libraries.allLibraries")}
        </span>
      </button>
    </>
  );
}

interface LibraryDrag {
  readonly handlersFor: (id: string) => DragHandlers;
  readonly classFor: (id: string) => string;
}

// The dragged id stays in component state rather than in the data transfer: browsers hide
// the transferred data during dragover, and the drop target needs it for its highlight.
function useLibraryDrag(moveTo: (id: string, targetId: string) => void): LibraryDrag {
  const [dragged, setDragged] = useState<string | null>(null);
  const [target, setTarget] = useState<string | null>(null);
  const end = () => {
    setDragged(null);
    setTarget(null);
  };
  return {
    handlersFor: (id) => ({
      draggable: true,
      onDragStart: (event) => {
        event.dataTransfer.effectAllowed = "move";
        setDragged(id);
      },
      onDragOver: (event) => {
        if (dragged === null) {
          return;
        }
        // Cancelling dragover is what allows a drop here.
        event.preventDefault();
        event.dataTransfer.dropEffect = "move";
        setTarget(id);
      },
      onDrop: (event) => {
        event.preventDefault();
        if (dragged !== null && dragged !== id) {
          moveTo(dragged, id);
        }
        end();
      },
      onDragLeave: () => {
        setTarget((current) => (current === id ? null : current));
      },
      onDragEnd: end,
    }),
    classFor: (id) => {
      if (id === dragged) {
        return "opacity-40";
      }
      return id === target ? "ring-2 ring-accent" : "";
    },
  };
}

/** Props of {@link AllLibraries}. */
export interface AllLibrariesProps {
  /** Every library in display order. */
  readonly links: readonly SidebarLibraryLink[];
  readonly layout: SidebarLayoutControls;
  readonly variant?: ArrangeVariant;
  /** Called when a library link is followed. */
  readonly onNavigate?: (() => void) | undefined;
}

/** Every library with move and pin buttons, and the reset entry (SH-12 ... SH-14). */
export function AllLibraries({
  links,
  layout,
  variant = "compact",
  onNavigate,
}: AllLibrariesProps) {
  const { t } = useTranslation();
  const focus = useMoveButtonFocus();
  return (
    <>
      <ul className={listClass}>
        {links.map((library, index) => (
          <ArrangeRow
            key={library.id}
            library={library}
            canMoveUp={index > 0}
            canMoveDown={index < links.length - 1}
            layout={layout}
            focus={focus}
            variant={variant}
            onNavigate={onNavigate}
          />
        ))}
      </ul>
      <button type="button" onClick={layout.reset} className={`${entryClass(variant)} mt-1 w-full`}>
        <ResetIcon className="size-5 shrink-0" />
        <span className="truncate">{t("sidebar.libraries.resetLayout")}</span>
      </button>
    </>
  );
}

interface ArrangeRowProps {
  readonly library: SidebarLibraryLink;
  readonly canMoveUp: boolean;
  readonly canMoveDown: boolean;
  readonly layout: SidebarLayoutControls;
  readonly focus: MoveButtonFocus;
  readonly variant: ArrangeVariant;
  readonly onNavigate: (() => void) | undefined;
}

function ArrangeRow(props: ArrangeRowProps) {
  const { library, canMoveUp, canMoveDown, layout, variant } = props;
  const buttonClass = smallButtonClass(variant);
  const { t } = useTranslation();
  return (
    <li className="flex items-center gap-0.5">
      <LibraryLink
        collapsed={false}
        library={library}
        variant={variant}
        onNavigate={props.onNavigate}
      />
      <MoveButton {...props} offset={-1} enabled={canMoveUp} />
      <MoveButton {...props} offset={1} enabled={canMoveDown} />
      <button
        type="button"
        aria-pressed={library.pinned !== false}
        aria-label={t("sidebar.libraries.pin", { name: library.name })}
        onClick={() => {
          layout.togglePinned(library.id);
        }}
        className={buttonClass}
      >
        <PinIcon className="size-4" />
      </button>
    </li>
  );
}

interface MoveButtonProps extends ArrangeRowProps {
  readonly offset: -1 | 1;
  readonly enabled: boolean;
}

function MoveButton({ library, layout, focus, variant, offset, enabled }: MoveButtonProps) {
  const { t } = useTranslation();
  const labelKey = offset < 0 ? "sidebar.libraries.moveUp" : "sidebar.libraries.moveDown";
  return (
    <button
      type="button"
      ref={focus.register(library.id, offset)}
      disabled={!enabled}
      aria-label={t(labelKey, { name: library.name })}
      onClick={() => {
        focus.request(library.id, offset);
        layout.move(library.id, offset);
      }}
      className={smallButtonClass(variant)}
    >
      {offset < 0 ? <ChevronUpIcon className="size-4" /> : <ChevronDownIcon className="size-4" />}
    </button>
  );
}

interface MoveButtonFocus {
  readonly register: (id: string, offset: -1 | 1) => (button: HTMLButtonElement | null) => void;
  readonly request: (id: string, offset: -1 | 1) => void;
}

// Reordering moves the row's DOM node, which drops keyboard focus in browsers. After the
// re-render focus returns to the pressed button, or to its counterpart when the library
// reached an end and the pressed one is now disabled.
function useMoveButtonFocus(): MoveButtonFocus {
  const buttons = useRef(new Map<string, HTMLButtonElement>());
  const pending = useRef<{ readonly id: string; readonly offset: -1 | 1 } | null>(null);
  const keyOf = (id: string, offset: number) => `${String(offset)}:${id}`;
  useEffect(() => {
    const request = pending.current;
    if (request === null) {
      return;
    }
    pending.current = null;
    const pressed = buttons.current.get(keyOf(request.id, request.offset));
    const counterpart = buttons.current.get(keyOf(request.id, -request.offset));
    (pressed?.disabled === false ? pressed : counterpart)?.focus();
  });
  return {
    register: (id, offset) => (button) => {
      const key = keyOf(id, offset);
      if (button === null) {
        buttons.current.delete(key);
      } else {
        buttons.current.set(key, button);
      }
    },
    request: (id, offset) => {
      pending.current = { id, offset };
    },
  };
}
